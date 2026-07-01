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
