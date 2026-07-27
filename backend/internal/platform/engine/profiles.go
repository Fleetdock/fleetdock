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
		return a.grantPostgresReadonly(ctx, p, user, database)
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
		return a.grantPostgresReadwrite(ctx, p, user, database)
	default:
		return fmt.Errorf("engine does not support profiles")
	}
}

func applyAdmin(ctx context.Context, admin Admin, p ConnParams, user, host, database string) error {
	switch a := admin.(type) {
	case *MariaDB:
		return a.grantMySQLProfile(ctx, p, user, host, database, []string{"ALL PRIVILEGES"})
	case *Postgres:
		return a.grantPostgresAdmin(ctx, p, user, database)
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

func (pg *Postgres) grantPostgresReadonly(ctx context.Context, p ConnParams, user, database string) error {
	if !identRe.MatchString(database) {
		return fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	role := quotePGIdent(user)
	db := quotePGIdent(database)
	stmts := []string{
		fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s", db, role),
		fmt.Sprintf("GRANT USAGE ON SCHEMA public TO %s", role),
		fmt.Sprintf("GRANT SELECT ON ALL TABLES IN SCHEMA public TO %s", role),
		fmt.Sprintf("GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO %s", role),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO %s", role),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON SEQUENCES TO %s", role),
	}
	for _, s := range stmts {
		if _, err := conn.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (pg *Postgres) grantPostgresReadwrite(ctx context.Context, p ConnParams, user, database string) error {
	if !identRe.MatchString(database) {
		return fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	role := quotePGIdent(user)
	db := quotePGIdent(database)
	stmts := []string{
		fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s", db, role),
		fmt.Sprintf("GRANT USAGE, CREATE ON SCHEMA public TO %s", role),
		fmt.Sprintf("GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON ALL TABLES IN SCHEMA public TO %s", role),
		fmt.Sprintf("GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO %s", role),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON TABLES TO %s", role),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO %s", role),
	}
	for _, s := range stmts {
		if _, err := conn.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (pg *Postgres) grantPostgresAdmin(ctx context.Context, p ConnParams, user, database string) error {
	if !identRe.MatchString(database) {
		return fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	role := quotePGIdent(user)
	db := quotePGIdent(database)
	stmts := []string{
		fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s", db, role),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON SCHEMA public TO %s", role),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO %s", role),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO %s", role),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO %s", role),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO %s", role),
	}
	for _, s := range stmts {
		if _, err := conn.Exec(ctx, s); err != nil {
			return err
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
