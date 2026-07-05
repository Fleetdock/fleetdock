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

	"fmt"

	agentapp "github.com/mariadb-cp/db-manager/backend/internal/app/agent"
	auditapp "github.com/mariadb-cp/db-manager/backend/internal/app/audit"
	authapp "github.com/mariadb-cp/db-manager/backend/internal/app/auth"
	backupapp "github.com/mariadb-cp/db-manager/backend/internal/app/backup"
	databaseapp "github.com/mariadb-cp/db-manager/backend/internal/app/database"
	dbadminapp "github.com/mariadb-cp/db-manager/backend/internal/app/dbadmin"
	destinationapp "github.com/mariadb-cp/db-manager/backend/internal/app/destination"
	instanceapp "github.com/mariadb-cp/db-manager/backend/internal/app/instance"
	notificationapp "github.com/mariadb-cp/db-manager/backend/internal/app/notification"
	operationapp "github.com/mariadb-cp/db-manager/backend/internal/app/operation"
	scheduleapp "github.com/mariadb-cp/db-manager/backend/internal/app/schedule"
	secretsapp "github.com/mariadb-cp/db-manager/backend/internal/app/secrets"
	serverapp "github.com/mariadb-cp/db-manager/backend/internal/app/server"
	summaryapp "github.com/mariadb-cp/db-manager/backend/internal/app/summary"
	tokenapp "github.com/mariadb-cp/db-manager/backend/internal/app/token"
	userapp "github.com/mariadb-cp/db-manager/backend/internal/app/user"
	"github.com/mariadb-cp/db-manager/backend/internal/config"
	"github.com/mariadb-cp/db-manager/backend/internal/infra/postgres"
	"github.com/mariadb-cp/db-manager/backend/internal/interfaces/httpapi"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/auth"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/crypto"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/notify"
	"github.com/mariadb-cp/db-manager/backend/internal/worker"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(); err != nil {
		slog.Error("fatal", "error", err.Error())
		os.Exit(1)
	}
}

// checkSecrets warns about (dev) or refuses (production) insecure defaults.
func checkSecrets(cfg config.Config) error {
	insecure := map[string]bool{
		"MDCP_JWT_SECRET":     cfg.JWTSecret == "dev-insecure-change-me" || cfg.JWTSecret == "change-me-in-production",
		"MDCP_ENCRYPTION_KEY": cfg.EncryptionKey == "dev-insecure-encryption-key" || cfg.EncryptionKey == "change-me-in-production",
		"MDCP_ADMIN_PASSWORD": cfg.AdminPassword == "admin12345",
	}
	for name, bad := range insecure {
		if !bad {
			continue
		}
		if cfg.IsProduction() {
			return fmt.Errorf("refusing to start: %s uses an insecure default (MDCP_ENV=production)", name)
		}
		slog.Warn("insecure default in use; set a strong value before production", "var", name)
	}
	return nil
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := checkSecrets(cfg); err != nil {
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
	auditRepo := postgres.NewAuditRepository(pool)
	notifRepo := postgres.NewNotificationRepository(pool)
	statsRepo := postgres.NewStatsRepository(pool)

	// Use cases (application services).
	jwt := auth.NewJWT(cfg.JWTSecret, cfg.JWTTTL)
	authSvc := authapp.NewService(userRepo, tokenRepo, jwt)
	tokenSvc := tokenapp.NewService(tokenRepo)
	serverSvc := serverapp.NewService(serverRepo)
	secretsSvc := secretsapp.NewService(secretRepo, crypto.NewEncryptor(cfg.EncryptionKey, "master-1"))
	opsSvc := operationapp.NewService(jobRepo, instanceRepo, databaseRepo, backupRepo, destRepo, secretsSvc)
	instanceSvc := instanceapp.NewService(instanceRepo, databaseRepo, secretsSvc, opsSvc)
	databaseSvc := databaseapp.NewService(databaseRepo, instanceRepo, opsSvc)
	agentSvc := agentapp.NewService(serverRepo, regTokenRepo)
	destSvc := destinationapp.NewService(destRepo, secretsSvc)
	backupSvc := backupapp.NewService(backupRepo, databaseRepo, instanceRepo, destRepo, opsSvc)
	scheduleSvc := scheduleapp.NewService(scheduleRepo, databaseRepo, destRepo, backupSvc)
	userSvc := userapp.NewService(userRepo)
	dbadminSvc := dbadminapp.NewService(instanceRepo, databaseRepo, serverRepo, secretsSvc)
	auditSvc := auditapp.NewService(auditRepo)
	summarySvc := summaryapp.NewService(statsRepo)
	notifSender := notify.New(notify.SMTPConfig{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword, From: cfg.SMTPFrom,
	})
	notifSvc := notificationapp.NewService(notifRepo, notifSender, agentSvc)
	opsSvc.SetNotifier(notifSvc)

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
		Instances:     httpapi.NewInstanceHandler(instanceSvc),
		Databases:     httpapi.NewDatabaseHandler(databaseSvc),
		Tokens:        httpapi.NewTokenHandler(tokenSvc),
		Users:         httpapi.NewUserHandler(userSvc),
		Operations:    httpapi.NewOperationHandler(opsSvc),
		Backups:       httpapi.NewBackupHandler(backupSvc),
		Schedules:     httpapi.NewScheduleHandler(scheduleSvc),
		Destinations:  httpapi.NewDestinationHandler(destSvc),
		DBAdmin:       httpapi.NewDBAdminHandler(dbadminSvc),
		Agents:        httpapi.NewAgentHandler(agentSvc, opsSvc),
		RegTokens:     httpapi.NewRegTokenHandler(agentSvc, cfg.PublicURL),
		Install:       httpapi.NewInstallHandler(cfg.PublicURL, cfg.AgentBinDir),
		Audit:         httpapi.NewAuditHandler(auditSvc),
		Notifications: httpapi.NewNotificationHandler(notifSvc),
		Overview:      httpapi.NewOverviewHandler(summarySvc, agentSvc),
		Authn:         httpapi.NewAuthenticator(authSvc),
		AuditRecorder: auditSvc,
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
