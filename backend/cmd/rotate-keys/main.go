// Command rotate-keys re-wraps every stored secret's data key under the current
// primary encryption key, so FLEETDOCK_ENCRYPTION_KEY can be rotated without losing
// data. Only the wrapped data key is re-encrypted — payload ciphertext is never
// touched or exposed.
//
// Usage (during a maintenance window):
//
//	# 1. keep the old key readable, promote a NEW key with a NEW id
//	export FLEETDOCK_ENCRYPTION_KEYS_OLD="master-1=<old-secret>"
//	export FLEETDOCK_ENCRYPTION_KEY="<new-secret>"
//	export FLEETDOCK_ENCRYPTION_KEY_ID="master-2"
//	# 2. run the rotation
//	go run ./cmd/rotate-keys   # or: make rotate-keys
//	# 3. once it reports 0 remaining, drop FLEETDOCK_ENCRYPTION_KEYS_OLD
//
// Rotation requires a NEW key id: secrets already stamped with the primary id
// are skipped, so reusing the same id with a different secret would silently
// leave them unreadable.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	secretsapp "github.com/TajBrains/fleetdock/backend/internal/app/secrets"
	"github.com/TajBrains/fleetdock/backend/internal/config"
	"github.com/TajBrains/fleetdock/backend/internal/infra/postgres"
	"github.com/TajBrains/fleetdock/backend/internal/platform/crypto"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(); err != nil {
		slog.Error("rotate-keys failed", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	encryptor, err := crypto.NewKeyring(cfg.EncryptionKeyID, cfg.EncryptionKeyring())
	if err != nil {
		return err
	}
	svc := secretsapp.NewService(postgres.NewSecretRepository(pool), encryptor)

	slog.Info("rotating secrets", "primary_key_id", cfg.EncryptionKeyID)
	res, err := svc.Rotate(ctx)
	if err != nil {
		return err
	}
	slog.Info("rotation complete",
		"total", res.Total, "rewrapped", res.Rewrapped, "already_current", res.Skipped)
	return nil
}
