package config

import (
	"strings"
	"testing"
)

func TestValidateSecrets_ProductionRejectsDefaults(t *testing.T) {
	cfg := Config{
		Env:            "production",
		JWTSecret:      "dev-insecure-change-me",
		EncryptionKey:  "dev-insecure-encryption-key",
		AdminPassword:  "admin12345",
		DatabaseURL:    "postgres://localhost/db",
	}

	err := cfg.ValidateSecrets(nil)
	if err == nil {
		t.Fatal("expected error in production with default secrets")
	}
	if !strings.Contains(err.Error(), "insecure default") {
		t.Fatalf("expected insecure default error, got: %v", err)
	}
}

func TestValidateSecrets_ProductionAcceptsStrongSecrets(t *testing.T) {
	cfg := Config{
		Env:            "production",
		JWTSecret:      "a-unique-production-jwt-secret",
		EncryptionKey:  "a-unique-production-encryption-key",
		AdminPassword:  "not-the-default-password",
		DatabaseURL:    "postgres://localhost/db",
	}

	if err := cfg.ValidateSecrets(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSecrets_DevelopmentWarnsOnDefaults(t *testing.T) {
	cfg := Config{
		Env:            "development",
		JWTSecret:      "dev-insecure-change-me",
		EncryptionKey:  "dev-insecure-encryption-key",
		AdminPassword:  "admin12345",
		DatabaseURL:    "postgres://localhost/db",
	}

	var warned []string
	if err := cfg.ValidateSecrets(func(name string) { warned = append(warned, name) }); err != nil {
		t.Fatalf("unexpected error in development: %v", err)
	}
	if len(warned) == 0 {
		t.Fatal("expected warnings for insecure defaults in development")
	}
}

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("FLEETDOCK_DATABASE_URL", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "FLEETDOCK_DATABASE_URL") {
		t.Fatalf("expected missing database URL error, got: %v", err)
	}
}
