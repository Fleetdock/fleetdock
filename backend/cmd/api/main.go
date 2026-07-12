// Command api is the entrypoint for the Fleetdock control-plane API.
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

	agentapp "github.com/TajBrains/fleetdock/backend/internal/app/agent"
	authapp "github.com/TajBrains/fleetdock/backend/internal/app/auth"
	authzapp "github.com/TajBrains/fleetdock/backend/internal/app/authz"
	backupapp "github.com/TajBrains/fleetdock/backend/internal/app/backup"
	databaseapp "github.com/TajBrains/fleetdock/backend/internal/app/database"
	dbadminapp "github.com/TajBrains/fleetdock/backend/internal/app/dbadmin"
	destinationapp "github.com/TajBrains/fleetdock/backend/internal/app/destination"
	instanceapp "github.com/TajBrains/fleetdock/backend/internal/app/instance"
	moveapp "github.com/TajBrains/fleetdock/backend/internal/app/move"
	notificationapp "github.com/TajBrains/fleetdock/backend/internal/app/notification"
	operationapp "github.com/TajBrains/fleetdock/backend/internal/app/operation"
	scheduleapp "github.com/TajBrains/fleetdock/backend/internal/app/schedule"
	secretsapp "github.com/TajBrains/fleetdock/backend/internal/app/secrets"
	serverapp "github.com/TajBrains/fleetdock/backend/internal/app/server"
	summaryapp "github.com/TajBrains/fleetdock/backend/internal/app/summary"
	tokenapp "github.com/TajBrains/fleetdock/backend/internal/app/token"
	userapp "github.com/TajBrains/fleetdock/backend/internal/app/user"
	"github.com/TajBrains/fleetdock/backend/internal/config"
	"github.com/TajBrains/fleetdock/backend/internal/infra/postgres"
	"github.com/TajBrains/fleetdock/backend/internal/interfaces/httpapi"
	"github.com/TajBrains/fleetdock/backend/internal/platform/auth"
	"github.com/TajBrains/fleetdock/backend/internal/platform/crypto"
	"github.com/TajBrains/fleetdock/backend/internal/platform/notify"
	"github.com/TajBrains/fleetdock/backend/internal/worker"
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
	if err := cfg.ValidateSecrets(func(name string) {
		slog.Warn("insecure default in use; set a strong value before production", "var", name)
	}); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("starting", "env", cfg.Env)

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
	secretRepo := postgres.NewSecretRepository(pool)
	jobRepo := postgres.NewJobRepository(pool)
	regTokenRepo := postgres.NewRegTokenRepository(pool)
	backupRepo := postgres.NewBackupRepository(pool)
	destRepo := postgres.NewBackupDestRepository(pool)
	scheduleRepo := postgres.NewScheduleRepository(pool)
	notifRepo := postgres.NewNotificationRepository(pool)
	statsRepo := postgres.NewStatsRepository(pool)
	authzRepo := postgres.NewAuthzRepository(pool)

	// Use cases (application services).
	jwt := auth.NewJWT(cfg.JWTSecret, cfg.JWTTTL)
	authSvc := authapp.NewService(userRepo, tokenRepo, jwt)
	resolver := authzapp.NewResolver(authzRepo)
	tokenSvc := tokenapp.NewService(tokenRepo)
	serverSvc := serverapp.NewService(serverRepo)
	encryptor, err := crypto.NewKeyring(cfg.EncryptionKeyID, cfg.EncryptionKeyring())
	if err != nil {
		return err
	}
	secretsSvc := secretsapp.NewService(secretRepo, encryptor)
	opsSvc := operationapp.NewService(jobRepo, instanceRepo, databaseRepo, backupRepo, destRepo, secretsSvc)
	instanceSvc := instanceapp.NewService(instanceRepo, databaseRepo, secretsSvc, opsSvc)
	databaseSvc := databaseapp.NewService(databaseRepo, instanceRepo, opsSvc)
	agentSvc := agentapp.NewService(serverRepo, regTokenRepo)
	destSvc := destinationapp.NewService(destRepo, secretsSvc)
	backupSvc := backupapp.NewService(backupRepo, databaseRepo, instanceRepo, destRepo, opsSvc)
	scheduleSvc := scheduleapp.NewService(scheduleRepo, databaseRepo, destRepo, backupSvc)
	userSvc := userapp.NewService(userRepo)
	dbadminSvc := dbadminapp.NewService(instanceRepo, databaseRepo, serverRepo, secretsSvc)
	summarySvc := summaryapp.NewService(statsRepo)
	notifSender := notify.New(notify.SMTPConfig{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword, From: cfg.SMTPFrom,
	})
	notifSvc := notificationapp.NewService(notifRepo, notifSender, agentSvc)
	opsSvc.SetNotifier(notifSvc)
	moveSvc := moveapp.NewService(databaseRepo, instanceRepo, backupSvc, databaseSvc)
	opsSvc.SetMover(moveSvc)

	// One-time admin bootstrap.
	if created, err := authSvc.EnsureAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return err
	} else if created {
		slog.Info("bootstrapped admin account", "email", cfg.AdminEmail)
	}

	// Background worker: control-plane operations, offline detection,
	// scheduled backups, retention, alert evaluation + notification dispatch.
	if cfg.WorkerEnabled {
		go worker.New(worker.Deps{
			Ops:              opsSvc,
			Agents:           agentSvc,
			Schedules:        scheduleSvc,
			Notifications:    notifSvc,
			HeartbeatTimeout: cfg.HeartbeatTimeout,
			MetricsRetention: cfg.MetricsRetention,
		}).Run(ctx)
	}

	// HTTP layer.
	router := httpapi.NewRouter(httpapi.RouterDeps{
		Auth:          httpapi.NewAuthHandler(authSvc),
		Servers:       httpapi.NewServerHandler(serverSvc),
		Instances:     httpapi.NewInstanceHandler(instanceSvc, resolver),
		Databases:     httpapi.NewDatabaseHandler(databaseSvc, resolver),
		Tokens:        httpapi.NewTokenHandler(tokenSvc),
		Users:         httpapi.NewUserHandler(userSvc),
		Operations:    httpapi.NewOperationHandler(opsSvc),
		Backups:       httpapi.NewBackupHandler(backupSvc, resolver),
		Schedules:     httpapi.NewScheduleHandler(scheduleSvc),
		Moves:         httpapi.NewMoveHandler(moveSvc, resolver),
		Destinations:  httpapi.NewDestinationHandler(destSvc),
		DBAdmin:       httpapi.NewDBAdminHandler(dbadminSvc),
		Agents:        httpapi.NewAgentHandler(agentSvc, opsSvc),
		RegTokens:     httpapi.NewRegTokenHandler(agentSvc, cfg.PublicURL),
		Install:       httpapi.NewInstallHandler(cfg.PublicURL, cfg.AgentBinDir),
		Notifications: httpapi.NewNotificationHandler(notifSvc),
		Overview:      httpapi.NewOverviewHandler(summarySvc, agentSvc),
		Docs:          httpapi.NewDocsHandler(),
		Authn:         httpapi.NewAuthenticator(authSvc),
		Resolver:      resolver,
		CORSOrigin:    cfg.CORSOrigin,
		Ready:         pool.Ping,
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
