// Package config loads runtime configuration from the environment.
//
// All variables use the FLEETDOCK_* prefix. FLEETDOCK_DATABASE_URL is required.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/gateway"
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

	// TrustProxyHeaders makes the API derive the client IP from the
	// X-Forwarded-For header (used for login rate limiting). Enable it only
	// when the API sits behind a trusted reverse proxy that sets the header;
	// otherwise clients could spoof it to evade rate limiting.
	TrustProxyHeaders bool

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

	// Gateway configures external database access via HAProxy.
	GatewayEnabled    bool
	GatewayPublicHost string
	GatewayPortStart  int
	GatewayPortEnd    int
	GatewayConfigPath string
	// GatewayMasterSock is the master CLI socket used to trigger reloads.
	GatewayMasterSock string
	// GatewayAdminSock is the stats socket used to read backend health and the
	// number of connections rejected by endpoint allowlists.
	GatewayAdminSock string
	// GatewayDiagPort serves a plaintext endpoint reporting the source address
	// HAProxy observes, so users can discover the address to allowlist. Zero
	// disables it.
	GatewayDiagPort int
	// GatewaySourceIPMode is "direct" or "proxy-protocol". Behind an L4 load
	// balancer, only proxy-protocol yields real client addresses.
	GatewaySourceIPMode string
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
	cfg := Config{
		HTTPAddr:        getenv("FLEETDOCK_HTTP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("FLEETDOCK_DATABASE_URL"),
		ShutdownTimeout: getenvDuration("FLEETDOCK_SHUTDOWN_TIMEOUT", 15*time.Second),
		RequestTimeout:  getenvDuration("FLEETDOCK_REQUEST_TIMEOUT", 30*time.Second),
		RunMigrations:   getenvBool("FLEETDOCK_RUN_MIGRATIONS", true),

		JWTSecret: getenv("FLEETDOCK_JWT_SECRET", "dev-insecure-change-me"),
		JWTTTL:    getenvDuration("FLEETDOCK_JWT_TTL", 24*time.Hour),

		AdminEmail:    getenv("FLEETDOCK_ADMIN_EMAIL", "admin@example.com"),
		AdminPassword: getenv("FLEETDOCK_ADMIN_PASSWORD", "admin12345"),

		CORSOrigin:        getenv("FLEETDOCK_CORS_ORIGIN", "http://localhost:3000"),
		TrustProxyHeaders: getenvBool("FLEETDOCK_TRUST_PROXY_HEADERS", false),

		EncryptionKey:     getenv("FLEETDOCK_ENCRYPTION_KEY", "dev-insecure-encryption-key"),
		EncryptionKeyID:   getenv("FLEETDOCK_ENCRYPTION_KEY_ID", "master-1"),
		EncryptionKeysOld: parseKeyList(os.Getenv("FLEETDOCK_ENCRYPTION_KEYS_OLD")),
		PublicURL:         getenv("FLEETDOCK_PUBLIC_URL", "http://localhost:8080"),
		AgentBinDir:       getenv("FLEETDOCK_AGENT_BIN_DIR", "/opt/fleetdock/agents"),
		WorkerEnabled:     getenvBool("FLEETDOCK_WORKER_ENABLED", true),
		HeartbeatTimeout:  getenvDuration("FLEETDOCK_HEARTBEAT_TIMEOUT", 2*time.Minute),
		MetricsRetention:  getenvDuration("FLEETDOCK_METRICS_RETENTION", 7*24*time.Hour),

		SMTPHost:     os.Getenv("FLEETDOCK_SMTP_HOST"),
		SMTPPort:     getenv("FLEETDOCK_SMTP_PORT", "587"),
		SMTPUsername: os.Getenv("FLEETDOCK_SMTP_USERNAME"),
		SMTPPassword: os.Getenv("FLEETDOCK_SMTP_PASSWORD"),
		SMTPFrom:     getenv("FLEETDOCK_SMTP_FROM", "fleetdock@localhost"),

		Env: getenv("FLEETDOCK_ENV", "development"),

		GatewayEnabled:      getenvBool("FLEETDOCK_GATEWAY_ENABLED", false),
		GatewayPublicHost:   getenv("FLEETDOCK_GATEWAY_PUBLIC_HOST", "gateway.localhost"),
		GatewayPortStart:    getenvInt("FLEETDOCK_GATEWAY_PORT_RANGE_START", 15432),
		GatewayPortEnd:      getenvInt("FLEETDOCK_GATEWAY_PORT_RANGE_END", 15481),
		GatewayConfigPath:   getenv("FLEETDOCK_GATEWAY_CONFIG_PATH", "/var/lib/fleetdock/gateway/haproxy.cfg"),
		GatewayMasterSock:   getenv("FLEETDOCK_GATEWAY_MASTER_SOCKET", "/var/lib/fleetdock/gateway/haproxy-master.sock"),
		GatewayAdminSock:    getenv("FLEETDOCK_GATEWAY_ADMIN_SOCKET", "/var/lib/fleetdock/gateway/haproxy-admin.sock"),
		GatewayDiagPort:     getenvInt("FLEETDOCK_GATEWAY_DIAG_PORT", 15431),
		GatewaySourceIPMode: getenv("FLEETDOCK_GATEWAY_SOURCE_IP_MODE", gateway.SourceIPDirect),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: FLEETDOCK_DATABASE_URL is required")
	}
	if cfg.GatewayEnabled {
		if cfg.GatewayPublicHost == "" {
			return Config{}, fmt.Errorf("config: FLEETDOCK_GATEWAY_PUBLIC_HOST is required when gateway is enabled")
		}
		if cfg.GatewayPortStart < 1 || cfg.GatewayPortEnd > 65535 || cfg.GatewayPortStart > cfg.GatewayPortEnd {
			return Config{}, fmt.Errorf("config: invalid gateway port range")
		}
		if cfg.GatewayConfigPath == "" {
			return Config{}, fmt.Errorf("config: FLEETDOCK_GATEWAY_CONFIG_PATH is required when gateway is enabled")
		}
		// Without the master socket a generated config is written and never
		// applied, so endpoints would look healthy while nothing is listening.
		if cfg.GatewayMasterSock == "" {
			return Config{}, fmt.Errorf("config: FLEETDOCK_GATEWAY_MASTER_SOCKET is required when gateway is enabled")
		}
		if cfg.GatewaySourceIPMode != gateway.SourceIPDirect && cfg.GatewaySourceIPMode != gateway.SourceIPProxyProtocol {
			return Config{}, fmt.Errorf("config: FLEETDOCK_GATEWAY_SOURCE_IP_MODE must be %q or %q",
				gateway.SourceIPDirect, gateway.SourceIPProxyProtocol)
		}
		if cfg.GatewayDiagPort != 0 &&
			cfg.GatewayDiagPort >= cfg.GatewayPortStart && cfg.GatewayDiagPort <= cfg.GatewayPortEnd {
			return Config{}, fmt.Errorf(
				"config: FLEETDOCK_GATEWAY_DIAG_PORT (%d) must fall outside the endpoint port range %d-%d",
				cfg.GatewayDiagPort, cfg.GatewayPortStart, cfg.GatewayPortEnd)
		}
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
		return v == "1" || strings.EqualFold(v, "true")
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
