package main

import (
	"context"
	"fmt"
	"time"

	secretsapp "github.com/Fleetdock/fleetdock/backend/internal/app/secrets"
	"github.com/Fleetdock/fleetdock/backend/internal/config"
	"github.com/Fleetdock/fleetdock/backend/internal/infra/postgres"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/crypto"
)

// rotateKeys implements `api rotate-keys`: it re-wraps every stored secret's
// data key under the current primary encryption key, so
// FLEETDOCK_ENCRYPTION_KEY can be rotated without losing data. Only the
// wrapped data keys change; payload ciphertext is never touched or exposed.
//
//	FLEETDOCK_ENCRYPTION_KEYS_OLD="master-1=<old secret>"   # keep the old key readable
//	FLEETDOCK_ENCRYPTION_KEY="<new secret>"                  # promote a new key...
//	FLEETDOCK_ENCRYPTION_KEY_ID="master-2"                   # ...under a NEW id
//	api rotate-keys
//
// Once it reports every secret current, FLEETDOCK_ENCRYPTION_KEYS_OLD can go.
// `fleetdock rotate-keys` does all of this for an install.sh install.
func rotateKeys() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.EncryptionKeysOld) == 0 {
		return fmt.Errorf("FLEETDOCK_ENCRYPTION_KEYS_OLD is empty: set it to <old key id>=<old key> so existing secrets stay readable")
	}
	if _, clash := cfg.EncryptionKeysOld[cfg.EncryptionKeyID]; clash {
		return fmt.Errorf("FLEETDOCK_ENCRYPTION_KEY_ID %q is also an old key id: the new key needs a new id", cfg.EncryptionKeyID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to the metadata database: %w", err)
	}
	defer pool.Close()

	encryptor, err := crypto.NewKeyring(cfg.EncryptionKeyID, cfg.EncryptionKeyring())
	if err != nil {
		return err
	}
	res, err := secretsapp.NewService(postgres.NewSecretRepository(pool), encryptor).Rotate(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Re-wrapped %d of %d secrets under key %q (%d were already current).\n",
		res.Rewrapped, res.Total, cfg.EncryptionKeyID, res.Skipped)
	return nil
}
