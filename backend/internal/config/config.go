// Package config loads runtime configuration from the environment.
//
// Variables use the FLEETDOCK_* prefix. The legacy MDCP_* names are still read
// as aliases for backward compatibility during upgrades.
package config

import (
	"fmt"
	"strings"
	"time"
)

// Config holds all runtime configuration for the API service.
type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	ShutdownTimeout time.Duration
	RequestTimeout  time.Duration
	RunMigrations   bool

	// Auth
	JWTSecret string
	JWTTTL    time.Duration

	// Admin bootstrap (used only when there are no users yet)
	AdminEmail    string
	AdminPassword string

	// CORS origin for the web frontend
	CORSOrigin string

	// EncryptionKey protects secrets at rest (instance credentials, S3 keys).
	EncryptionKey string
	// EncryptionKeyID names the primary key that wraps new data keys.
	EncryptionKeyID string
	// EncryptionKeysOld holds retired keys still needed to decrypt existing
	// secrets during a rotation window, as id=secret pairs.
	EncryptionKeysOld map[string]string
	// PublicURL is the externally reachable base URL of this API (used in
	// the agent install command).
	PublicURL string
	// AgentBinDir holds cross-compiled agent binaries served to installers.
	AgentBinDir string
	// WorkerEnabled runs the in-process operations worker (external
	// instances, offline detection).
	WorkerEnabled bool
	// HeartbeatTimeout is how long without a heartbeat before a server is
	// marked offline.
	HeartbeatTimeout time.Duration
	// MetricsRetention is how long server health-history samples are kept.
	MetricsRetention time.Duration

	// SMTP configures outbound email for notification channels. When
	// SMTPHost is empty, email delivery is disabled.
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	// Env is the deployment environment ("development" or "production").
	// In production the server refuses to start with insecure defaults.
	Env string
}

// IsProduction reports whether the server runs in production mode.
func (c Config) IsProduction() bool { return c.Env == "production" }

// ValidateSecrets warns about (dev) or refuses (production) insecure defaults.
func (c Config) ValidateSecrets(warn func(name string)) error {
	insecure := map[string]bool{
		"FLEETDOCK_JWT_SECRET":     c.JWTSecret == "dev-insecure-change-me" || c.JWTSecret == "change-me-in-production",
		"FLEETDOCK_ENCRYPTION_KEY": c.EncryptionKey == "dev-insecure-encryption-key" || c.EncryptionKey == "change-me-in-production",
		"FLEETDOCK_ADMIN_PASSWORD": c.AdminPassword == "admin12345",
	}
	for name, bad := range insecure {
		if !bad {
			continue
		}
		if c.IsProduction() {
			return fmt.Errorf("refusing to start: %s uses an insecure default (FLEETDOCK_ENV=production)", name)
		}
		if warn != nil {
			warn(name)
		}
	}
	return nil
}

// EncryptionKeyring returns every key id → secret needed to decrypt secrets:
// the primary key plus any retired keys. The primary key always wins on a
// collision.
func (c Config) EncryptionKeyring() map[string]string {
	m := make(map[string]string, len(c.EncryptionKeysOld)+1)
	for id, s := range c.EncryptionKeysOld {
		m[id] = s
	}
	m[c.EncryptionKeyID] = c.EncryptionKey
	return m
}

// parseKeyList parses "id=secret,id2=secret2" into a map.
func parseKeyList(raw string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		i := strings.IndexByte(pair, '=')
		if i <= 0 {
			continue
		}
		id := strings.TrimSpace(pair[:i])
		secret := strings.TrimSpace(pair[i+1:])
		if id != "" && secret != "" {
			out[id] = secret
		}
	}
	return out
}

// Load reads configuration from the environment and validates it.
func Load() (Config, error) {
	keysOld := env("FLEETDOCK_ENCRYPTION_KEYS_OLD", "MDCP_ENCRYPTION_KEYS_OLD", "")
	cfg := Config{
		HTTPAddr:        env("FLEETDOCK_HTTP_ADDR", "MDCP_HTTP_ADDR", ":8080"),
		DatabaseURL:     envRequired("FLEETDOCK_DATABASE_URL", "MDCP_DATABASE_URL"),
		ShutdownTimeout: envDuration("FLEETDOCK_SHUTDOWN_TIMEOUT", "MDCP_SHUTDOWN_TIMEOUT", 15*time.Second),
		RequestTimeout:  envDuration("FLEETDOCK_REQUEST_TIMEOUT", "MDCP_REQUEST_TIMEOUT", 30*time.Second),
		RunMigrations:   envBool("FLEETDOCK_RUN_MIGRATIONS", "MDCP_RUN_MIGRATIONS", true),

		JWTSecret: env("FLEETDOCK_JWT_SECRET", "MDCP_JWT_SECRET", "dev-insecure-change-me"),
		JWTTTL:    envDuration("FLEETDOCK_JWT_TTL", "MDCP_JWT_TTL", 24*time.Hour),

		AdminEmail:    env("FLEETDOCK_ADMIN_EMAIL", "MDCP_ADMIN_EMAIL", "admin@example.com"),
		AdminPassword: env("FLEETDOCK_ADMIN_PASSWORD", "MDCP_ADMIN_PASSWORD", "admin12345"),

		CORSOrigin: env("FLEETDOCK_CORS_ORIGIN", "MDCP_CORS_ORIGIN", "http://localhost:3000"),

		EncryptionKey:     env("FLEETDOCK_ENCRYPTION_KEY", "MDCP_ENCRYPTION_KEY", "dev-insecure-encryption-key"),
		EncryptionKeyID:   env("FLEETDOCK_ENCRYPTION_KEY_ID", "MDCP_ENCRYPTION_KEY_ID", "master-1"),
		EncryptionKeysOld: parseKeyList(keysOld),
		PublicURL:         env("FLEETDOCK_PUBLIC_URL", "MDCP_PUBLIC_URL", "http://localhost:8080"),
		AgentBinDir:       env("FLEETDOCK_AGENT_BIN_DIR", "MDCP_AGENT_BIN_DIR", "/opt/fleetdock/agents"),
		WorkerEnabled:     envBool("FLEETDOCK_WORKER_ENABLED", "MDCP_WORKER_ENABLED", true),
		HeartbeatTimeout:  envDuration("FLEETDOCK_HEARTBEAT_TIMEOUT", "MDCP_HEARTBEAT_TIMEOUT", 2*time.Minute),
		MetricsRetention:  envDuration("FLEETDOCK_METRICS_RETENTION", "MDCP_METRICS_RETENTION", 7*24*time.Hour),

		SMTPHost:     env("FLEETDOCK_SMTP_HOST", "MDCP_SMTP_HOST", ""),
		SMTPPort:     env("FLEETDOCK_SMTP_PORT", "MDCP_SMTP_PORT", "587"),
		SMTPUsername: env("FLEETDOCK_SMTP_USERNAME", "MDCP_SMTP_USERNAME", ""),
		SMTPPassword: env("FLEETDOCK_SMTP_PASSWORD", "MDCP_SMTP_PASSWORD", ""),
		SMTPFrom:     env("FLEETDOCK_SMTP_FROM", "MDCP_SMTP_FROM", "fleetdock@localhost"),

		Env: env("FLEETDOCK_ENV", "MDCP_ENV", "development"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: FLEETDOCK_DATABASE_URL is required (MDCP_DATABASE_URL is a deprecated alias)")
	}
	return cfg, nil
}
