// Command api is the entrypoint for the db-manager control-plane API.
//
// It wires configuration, the Postgres pool, migrations, repositories, use
// cases and the HTTP router (manual dependency injection), bootstraps an admin
// account on first run, then serves with graceful shutdown.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	authapp "github.com/mariadb-cp/db-manager/backend/internal/app/auth"
	databaseapp "github.com/mariadb-cp/db-manager/backend/internal/app/database"
	instanceapp "github.com/mariadb-cp/db-manager/backend/internal/app/instance"
	serverapp "github.com/mariadb-cp/db-manager/backend/internal/app/server"
	tokenapp "github.com/mariadb-cp/db-manager/backend/internal/app/token"
	"github.com/mariadb-cp/db-manager/backend/internal/config"
	"github.com/mariadb-cp/db-manager/backend/internal/infra/postgres"
	"github.com/mariadb-cp/db-manager/backend/internal/interfaces/httpapi"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/auth"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(); err != nil {
		slog.Error("fatal", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.JWTSecret == "dev-insecure-change-me" {
		slog.Warn("using insecure default MDCP_JWT_SECRET; set a strong secret in production")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.RunMigrations {
		if err := postgres.Migrate(ctx, pool); err != nil {
			return err
		}
		slog.Info("migrations applied")
	}

	// Repositories (infra adapters).
	userRepo := postgres.NewUserRepository(pool)
	tokenRepo := postgres.NewTokenRepository(pool)
	serverRepo := postgres.NewServerRepository(pool)
	instanceRepo := postgres.NewInstanceRepository(pool)
	databaseRepo := postgres.NewDatabaseRepository(pool)

	// Use cases (application services).
	jwt := auth.NewJWT(cfg.JWTSecret, cfg.JWTTTL)
	authSvc := authapp.NewService(userRepo, tokenRepo, jwt)
	tokenSvc := tokenapp.NewService(tokenRepo)
	serverSvc := serverapp.NewService(serverRepo)
	instanceSvc := instanceapp.NewService(instanceRepo)
	databaseSvc := databaseapp.NewService(databaseRepo)

	// One-time admin bootstrap.
	if created, err := authSvc.EnsureAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return err
	} else if created {
		slog.Info("bootstrapped admin account", "email", cfg.AdminEmail)
	}

	// HTTP layer.
	router := httpapi.NewRouter(httpapi.RouterDeps{
		Auth:       httpapi.NewAuthHandler(authSvc),
		Servers:    httpapi.NewServerHandler(serverSvc),
		Instances:  httpapi.NewInstanceHandler(instanceSvc),
		Databases:  httpapi.NewDatabaseHandler(databaseSvc),
		Tokens:     httpapi.NewTokenHandler(tokenSvc),
		Authn:      httpapi.NewAuthenticator(authSvc),
		CORSOrigin: cfg.CORSOrigin,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.RequestTimeout,
		WriteTimeout:      cfg.RequestTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
