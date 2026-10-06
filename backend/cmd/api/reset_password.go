package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/Fleetdock/fleetdock/backend/internal/infra/postgres"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/auth"
)

// resetPassword implements `api reset-password <email>`: the recovery path
// for a locked-out admin. It sets a new password (FLEETDOCK_NEW_PASSWORD, or a
// generated one printed to stdout), reactivates the account if it was
// suspended, and ends every existing browser session of that user.
//
// It talks to the metadata database directly and needs only
// FLEETDOCK_DATABASE_URL, so it works when the API itself will not start.
func resetPassword(args []string) error {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("usage: api reset-password <email>")
	}
	email := strings.ToLower(strings.TrimSpace(args[0]))
	dbURL := os.Getenv("FLEETDOCK_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("FLEETDOCK_DATABASE_URL is not set")
	}
	password := os.Getenv("FLEETDOCK_NEW_PASSWORD")
	generated := password == ""
	if generated {
		var err error
		if password, err = randomPassword(20); err != nil {
			return err
		}
	}
	if len(password) < 8 {
		return fmt.Errorf("the new password must be at least 8 characters")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.NewPool(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to the metadata database: %w", err)
	}
	defer pool.Close()

	tag, err := pool.Exec(ctx, `
		UPDATE users SET password_hash = $2, status = 'active',
			token_epoch = token_epoch + 1, version = version + 1
		WHERE lower(email) = $1`, email, hash)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no user with email %q", email)
	}
	if generated {
		fmt.Println(password)
	} else {
		fmt.Println("password updated")
	}
	return nil
}

func randomPassword(n int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, n)
	for i := range b {
		k, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[k.Int64()]
	}
	return string(b), nil
}
