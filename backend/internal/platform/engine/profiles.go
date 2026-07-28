package engine

import (
	"context"
	"fmt"
	"strings"
)

// AccessProfile is a reusable permission preset for application credentials.
type AccessProfile string

const (
	ProfileReadonly  AccessProfile = "readonly"
	ProfileReadWrite AccessProfile = "readwrite"
	ProfileAdmin     AccessProfile = "admin"
)

// ApplyProfile grants database-scoped privileges for a profile.
func ApplyProfile(ctx context.Context, admin Admin, p ConnParams, user, host, database string, profile AccessProfile) error {
	switch profile {
	case ProfileReadonly:
		return applyReadonly(ctx, admin, p, user, host, database)
	case ProfileReadWrite:
		return applyReadwrite(ctx, admin, p, user, host, database)
	case ProfileAdmin:
		return applyAdmin(ctx, admin, p, user, host, database)
	default:
		return fmt.Errorf("unsupported access profile %q", profile)
	}
}

func applyReadonly(ctx context.Context, admin Admin, p ConnParams, user, host, database string) error {
	switch a := admin.(type) {
	case *MariaDB:
		return a.grantMySQLProfile(ctx, p, user, host, database, []string{"SELECT", "SHOW VIEW"})
	case *Postgres:
		return a.applyPGProfile(ctx, p, user, database, ProfileReadonly)
	default:
		return fmt.Errorf("engine does not support profiles")
	}
}

func applyReadwrite(ctx context.Context, admin Admin, p ConnParams, user, host, database string) error {
	switch a := admin.(type) {
	case *MariaDB:
		return a.grantMySQLProfile(ctx, p, user, host, database, []string{
			"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER", "INDEX",
			"CREATE TEMPORARY TABLES", "LOCK TABLES", "EXECUTE", "CREATE VIEW", "SHOW VIEW",
			"CREATE ROUTINE", "ALTER ROUTINE", "TRIGGER", "REFERENCES",
		})
	case *Postgres:
		return a.applyPGProfile(ctx, p, user, database, ProfileReadWrite)
	default:
		return fmt.Errorf("engine does not support profiles")
	}
}

func applyAdmin(ctx context.Context, admin Admin, p ConnParams, user, host, database string) error {
	switch a := admin.(type) {
	case *MariaDB:
		return a.grantMySQLProfile(ctx, p, user, host, database, []string{"ALL PRIVILEGES"})
	case *Postgres:
		return a.applyPGProfile(ctx, p, user, database, ProfileAdmin)
	default:
		return fmt.Errorf("engine does not support profiles")
	}
}

func (m *MariaDB) grantMySQLProfile(ctx context.Context, p ConnParams, user, host, database string, privs []string) error {
	if err := m.Grant(ctx, p, user, host, database, privs); err != nil {
		return err
	}
	return nil
}

// pgProfileGrants is one profile expressed at PostgreSQL's four grant scopes.
type pgProfileGrants struct {
	database  string // privileges on the database itself
	schema    string // privileges on each schema
	tables    string // privileges on existing and future tables
	sequences string // privileges on existing and future sequences
}

var pgProfiles = map[AccessProfile]pgProfileGrants{
	ProfileReadonly: {
		database:  "CONNECT",
		schema:    "USAGE",
		tables:    "SELECT",
		sequences: "SELECT",
	},
	ProfileReadWrite: {
		database:  "CONNECT",
		schema:    "USAGE, CREATE",
		tables:    "SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER",
		sequences: "USAGE, SELECT, UPDATE",
	},
	ProfileAdmin: {
		database:  "ALL PRIVILEGES",
		schema:    "ALL PRIVILEGES",
		tables:    "ALL PRIVILEGES",
		sequences: "ALL PRIVILEGES",
	},
}

// applyPGProfile grants a profile across every schema in the database, not just
// public. A PostgreSQL database can hold many schemas, and a credential that
// only reaches public is silently useless against a database organised any
// other way.
//
// Two limits are inherent to PostgreSQL and worth knowing: the grant is a
// snapshot, so a schema created afterwards is not covered (there is no "all
// future schemas" default privilege), and ALTER DEFAULT PRIVILEGES only applies
// to objects later created by the role running these statements.
func (pg *Postgres) applyPGProfile(ctx context.Context, p ConnParams, user, database string, profile AccessProfile) error {
	g, ok := pgProfiles[profile]
	if !ok {
		return fmt.Errorf("unsupported access profile %q", profile)
	}
	if !validPGRole(user) {
		return fmt.Errorf("invalid role name")
	}
	if !identRe.MatchString(database) {
		return fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	role := quotePGIdent(user)
	if _, err := conn.Exec(ctx, fmt.Sprintf("GRANT %s ON DATABASE %s TO %s",
		g.database, quotePGIdent(database), role)); err != nil {
		return err
	}

	schemas, err := pg.userSchemas(ctx, conn)
	if err != nil {
		return err
	}
	for _, schema := range schemas {
		s := quotePGIdent(schema)
		stmts := []string{
			fmt.Sprintf("GRANT %s ON SCHEMA %s TO %s", g.schema, s, role),
			fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA %s TO %s", g.tables, s, role),
			fmt.Sprintf("GRANT %s ON ALL SEQUENCES IN SCHEMA %s TO %s", g.sequences, s, role),
			fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT %s ON TABLES TO %s", s, g.tables, role),
			fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT %s ON SEQUENCES TO %s", s, g.sequences, role),
		}
		for _, stmt := range stmts {
			if _, err := conn.Exec(ctx, stmt); err != nil {
				// Name the schema: "permission denied" on a schema owned by
				// another role is the likely cause and is otherwise opaque.
				return fmt.Errorf("granting %s on schema %q: %w", profile, schema, err)
			}
		}
	}
	return nil
}

// RotatePassword updates the password for an existing database user.
func RotatePassword(ctx context.Context, admin Admin, p ConnParams, user, host, password string) error {
	switch a := admin.(type) {
	case *MariaDB:
		if err := a.validateAccount(user, host); err != nil {
			return err
		}
		db, err := a.open(p)
		if err != nil {
			return err
		}
		defer db.Close()
		pw := escapeMySQLString(password)
		_, err = db.ExecContext(ctx, fmt.Sprintf(
			"ALTER USER %s IDENTIFIED BY '%s'", quoteAccount(user, host), pw))
		return err
	case *Postgres:
		if !validPGRole(user) {
			return fmt.Errorf("invalid role name")
		}
		conn, err := a.connect(ctx, p, "postgres")
		if err != nil {
			return err
		}
		defer conn.Close(ctx)
		pw := strings.ReplaceAll(password, "'", "''")
		_, err = conn.Exec(ctx, fmt.Sprintf(
			"ALTER ROLE %s WITH PASSWORD '%s'", quotePGIdent(user), pw))
		return err
	default:
		return fmt.Errorf("engine does not support password rotation")
	}
}
