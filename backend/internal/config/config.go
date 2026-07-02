// Package config loads runtime configuration from the environment.
//
// Nothing is hardcoded: every value comes from an environment variable with a
// sane default, except MDCP_DATABASE_URL which is mandatory.
package config

import (
	"fmt"
	"os"
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
}

// Load reads configuration from the environment and validates it.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        getenv("MDCP_HTTP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("MDCP_DATABASE_URL"),
		ShutdownTimeout: getenvDuration("MDCP_SHUTDOWN_TIMEOUT", 15*time.Second),
		RequestTimeout:  getenvDuration("MDCP_REQUEST_TIMEOUT", 30*time.Second),
		RunMigrations:   getenvBool("MDCP_RUN_MIGRATIONS", true),

		JWTSecret: getenv("MDCP_JWT_SECRET", "dev-insecure-change-me"),
		JWTTTL:    getenvDuration("MDCP_JWT_TTL", 24*time.Hour),

		AdminEmail:    getenv("MDCP_ADMIN_EMAIL", "admin@example.com"),
		AdminPassword: getenv("MDCP_ADMIN_PASSWORD", "admin12345"),

		CORSOrigin: getenv("MDCP_CORS_ORIGIN", "http://localhost:3000"),

		EncryptionKey:    getenv("MDCP_ENCRYPTION_KEY", "dev-insecure-encryption-key"),
		PublicURL:        getenv("MDCP_PUBLIC_URL", "http://localhost:8080"),
		AgentBinDir:      getenv("MDCP_AGENT_BIN_DIR", "/opt/db-manager/agents"),
		WorkerEnabled:    getenvBool("MDCP_WORKER_ENABLED", true),
		HeartbeatTimeout: getenvDuration("MDCP_HEARTBEAT_TIMEOUT", 2*time.Minute),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: MDCP_DATABASE_URL is required")
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || v == "true" || v == "TRUE"
	}
	return def
}
