package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateSecrets_ProductionRejectsDefaults(t *testing.T) {
	cfg := Config{
		Env:           "production",
		JWTSecret:     "dev-insecure-change-me",
		EncryptionKey: "dev-insecure-encryption-key",
		AdminPassword: "admin12345",
		DatabaseURL:   "postgres://localhost/db",
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
		Env:           "production",
		JWTSecret:     "a-unique-production-jwt-secret",
		EncryptionKey: "a-unique-production-encryption-key",
		AdminPassword: "not-the-default-password",
		DatabaseURL:   "postgres://localhost/db",
	}

	if err := cfg.ValidateSecrets(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSecrets_DevelopmentWarnsOnDefaults(t *testing.T) {
	cfg := Config{
		Env:           "development",
		JWTSecret:     "dev-insecure-change-me",
		EncryptionKey: "dev-insecure-encryption-key",
		AdminPassword: "admin12345",
		DatabaseURL:   "postgres://localhost/db",
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

func TestLoad_WorkerIntervalDefaults(t *testing.T) {
	t.Setenv("FLEETDOCK_DATABASE_URL", "postgres://localhost/db")
	clearWorkerIntervalEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.JobPollInterval != 3*time.Second {
		t.Errorf("JobPollInterval = %s, want 3s", cfg.JobPollInterval)
	}
	if cfg.AlertInterval != 15*time.Second {
		t.Errorf("AlertInterval = %s, want 15s", cfg.AlertInterval)
	}
	if cfg.HealthCheckInterval != time.Minute {
		t.Errorf("HealthCheckInterval = %s, want 1m", cfg.HealthCheckInterval)
	}
}

func TestLoad_WorkerIntervals(t *testing.T) {
	t.Setenv("FLEETDOCK_DATABASE_URL", "postgres://localhost/db")
	clearWorkerIntervalEnv(t)
	t.Setenv("FLEETDOCK_JOB_POLL_INTERVAL", "2s")
	t.Setenv("FLEETDOCK_ALERT_INTERVAL", "10s")
	t.Setenv("FLEETDOCK_HEALTH_CHECK_INTERVAL", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.JobPollInterval != 2*time.Second {
		t.Errorf("JobPollInterval = %s, want 2s", cfg.JobPollInterval)
	}
	if cfg.AlertInterval != 10*time.Second {
		t.Errorf("AlertInterval = %s, want 10s", cfg.AlertInterval)
	}
	if cfg.HealthCheckInterval != 30*time.Second {
		t.Errorf("HealthCheckInterval = %s, want 30s", cfg.HealthCheckInterval)
	}
}

func TestLoad_WorkerIntervalsRejectInvalidValues(t *testing.T) {
	for _, name := range []string{
		"FLEETDOCK_JOB_POLL_INTERVAL",
		"FLEETDOCK_ALERT_INTERVAL",
		"FLEETDOCK_HEALTH_CHECK_INTERVAL",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("FLEETDOCK_DATABASE_URL", "postgres://localhost/db")
			clearWorkerIntervalEnv(t)
			t.Setenv(name, "sometimes")

			_, err := Load()
			want := name + " must be a valid duration"
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("expected %q, got %v", want, err)
			}
		})
	}
}

func TestLoad_WorkerIntervalsEnforceMinimums(t *testing.T) {
	tests := []struct {
		name  string
		value string
		min   string
	}{
		{name: "FLEETDOCK_JOB_POLL_INTERVAL", value: "999ms", min: "1s"},
		{name: "FLEETDOCK_ALERT_INTERVAL", value: "4s", min: "5s"},
		{name: "FLEETDOCK_HEALTH_CHECK_INTERVAL", value: "14s", min: "15s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FLEETDOCK_DATABASE_URL", "postgres://localhost/db")
			clearWorkerIntervalEnv(t)
			t.Setenv(tt.name, tt.value)

			_, err := Load()
			want := tt.name + " must be at least " + tt.min
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("expected %q, got %v", want, err)
			}
		})
	}
}

func clearWorkerIntervalEnv(t *testing.T) {
	t.Helper()
	t.Setenv("FLEETDOCK_JOB_POLL_INTERVAL", "")
	t.Setenv("FLEETDOCK_ALERT_INTERVAL", "")
	t.Setenv("FLEETDOCK_HEALTH_CHECK_INTERVAL", "")
}

func TestValidateSecrets_ProductionRejectsShortSecrets(t *testing.T) {
	cfg := Config{
		Env:           "production",
		JWTSecret:     "short",
		EncryptionKey: "a-unique-production-encryption-key",
		AdminPassword: "not-the-default-password",
	}
	if err := cfg.ValidateSecrets(nil); err == nil || !strings.Contains(err.Error(), "FLEETDOCK_JWT_SECRET") {
		t.Fatalf("expected a short JWT secret to be refused, got %v", err)
	}
}

func TestMetadataDBAddr(t *testing.T) {
	cases := map[string]string{
		"postgres://u:p@postgres:5432/fleetdock?sslmode=disable": "postgres:5432",
		"postgres://u:p@db.example.com/fleetdock":                "db.example.com:5432",
		"postgresql://u@[::1]:6543/x":                            "[::1]:6543",
		"not a url":                                              "",
	}
	for in, want := range cases {
		if got := (Config{DatabaseURL: in}).MetadataDBAddr(); got != want {
			t.Errorf("MetadataDBAddr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateSecrets_ProductionRejectsDemoSecrets(t *testing.T) {
	cfg := Config{
		Env:           "production",
		JWTSecret:     "demo-only-jwt-secret-not-for-production-0000000000",
		EncryptionKey: "demo-only-encryption-key-not-for-production-00000",
		AdminPassword: "fleetdock-demo-password",
	}
	if err := cfg.ValidateSecrets(nil); err == nil || !strings.Contains(err.Error(), "insecure default") {
		t.Fatalf("production with the demo stack's secrets: %v", err)
	}
	cfg.Env = "demo"
	if err := cfg.ValidateSecrets(func(string) {}); err != nil {
		t.Fatalf("demo env: %v", err)
	}
}
