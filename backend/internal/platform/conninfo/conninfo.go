// Package conninfo builds database connection URLs and structured fields.
package conninfo

import (
	"fmt"
	"net/url"
	"strings"

	endpointdom "github.com/Fleetdock/fleetdock/backend/internal/domain/endpoint"
)

// Fields are structured connection parameters for clients that do not accept
// URLs. The password is deliberately absent: it is returned once at the top
// level of the reveal payload, and duplicating a secret in the same response
// only widens where it can leak.
type Fields struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Database string `json:"database"`
	SSLMode  string `json:"ssl_mode,omitempty"`
}

// BuildURL returns a connection URL with percent-encoded components.
func BuildURL(t endpointdom.Target, user, password, database string) (string, error) {
	user = strings.TrimSpace(user)
	host := strings.TrimSpace(t.Host)
	database = strings.TrimSpace(database)
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	if t.Port < 1 || t.Port > 65535 {
		return "", fmt.Errorf("invalid port")
	}
	scheme, err := scheme(t.Protocol)
	if err != nil {
		return "", err
	}

	u := &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", host, t.Port),
		Path:   "/" + database,
	}
	if user != "" {
		u.User = url.UserPassword(user, password)
	}
	// Only Postgres URLs carry TLS in the query string. The `ssl-mode`
	// parameter MySQL uses is understood by mysqlsh and little else, so for
	// MySQL/MariaDB the mode travels in Fields and CLICommand instead.
	if t.Protocol == endpointdom.ProtocolPostgreSQL {
		q := url.Values{}
		q.Set("sslmode", sslMode(t.Protocol, t.TLSMode))
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

// BuildFields returns structured connection parameters.
func BuildFields(t endpointdom.Target, user, database string) Fields {
	return Fields{
		Host:     t.Host,
		Port:     t.Port,
		User:     user,
		Database: database,
		SSLMode:  sslMode(t.Protocol, t.TLSMode),
	}
}

// CLICommand returns a ready-to-paste command for the engine's own client,
// which is what most people reach for first and what a bare URL does not cover
// for MySQL and MariaDB.
func CLICommand(t endpointdom.Target, user, database string) string {
	switch t.Protocol {
	case endpointdom.ProtocolPostgreSQL:
		return fmt.Sprintf("psql %q", fmt.Sprintf("postgresql://%s@%s:%d/%s?sslmode=%s",
			url.QueryEscape(user), t.Host, t.Port, database, sslMode(t.Protocol, t.TLSMode)))
	case endpointdom.ProtocolMySQL, endpointdom.ProtocolMariaDB:
		client := "mysql"
		if t.Protocol == endpointdom.ProtocolMariaDB {
			client = "mariadb"
		}
		return fmt.Sprintf("%s -h %s -P %d -u %s -p --ssl-mode=%s %s",
			client, t.Host, t.Port, user, sslMode(t.Protocol, t.TLSMode), database)
	default:
		return ""
	}
}

func scheme(protocol endpointdom.Protocol) (string, error) {
	switch protocol {
	case endpointdom.ProtocolPostgreSQL:
		return "postgresql", nil
	case endpointdom.ProtocolMySQL, endpointdom.ProtocolMariaDB:
		return "mysql", nil
	default:
		return "", fmt.Errorf("unsupported protocol %q", protocol)
	}
}

// sslMode renders the TLS mode in the spelling each engine's clients expect.
func sslMode(protocol endpointdom.Protocol, tlsMode endpointdom.TLSMode) string {
	if protocol == endpointdom.ProtocolPostgreSQL {
		switch tlsMode {
		case endpointdom.TLSRequired:
			return "require"
		case endpointdom.TLSPreferred:
			return "prefer"
		default:
			return "disable"
		}
	}
	switch tlsMode {
	case endpointdom.TLSRequired:
		return "REQUIRED"
	case endpointdom.TLSPreferred:
		return "PREFERRED"
	default:
		return "DISABLED"
	}
}

// MaskPassword replaces the password in a URL with asterisks.
func MaskPassword(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, set := u.User.Password(); set {
		u.User = url.UserPassword(u.User.Username(), "********")
	}
	return u.String()
}
