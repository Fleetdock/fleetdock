package config

import (
	"os"
	"strings"
	"time"
)

// env reads the primary FLEETDOCK_* variable, then the legacy MDCP_* alias.
func env(primary, legacy, def string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	if legacy != "" {
		if v := os.Getenv(legacy); v != "" {
			return v
		}
	}
	return def
}

func envRequired(primary, legacy string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	if legacy != "" {
		if v := os.Getenv(legacy); v != "" {
			return v
		}
	}
	return ""
}

func envDuration(primary, legacy string, def time.Duration) time.Duration {
	if v := env(primary, legacy, ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envBool(primary, legacy string, def bool) bool {
	v := env(primary, legacy, "")
	if v == "" {
		return def
	}
	return v == "1" || strings.EqualFold(v, "true")
}
