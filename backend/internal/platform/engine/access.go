package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

// IsPermissionDenied reports whether err is the engine refusing an operation
// for lack of privileges (as opposed to a syntax error, missing object, ...).
func IsPermissionDenied(err error) bool {
	var my *mysql.MySQLError
	if errors.As(err, &my) {
		switch my.Number {
		case 1044, // ER_DBACCESS_DENIED_ERROR
			1142, // ER_TABLEACCESS_DENIED_ERROR
			1143, // ER_COLUMNACCESS_DENIED_ERROR
			1227, // ER_SPECIFIC_ACCESS_DENIED_ERROR
			1370: // ER_PROCACCESS_DENIED_ERROR
			return true
		}
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code == "42501" // insufficient_privilege
	}
	return false
}

// IsLoginRejected reports whether err is the server refusing to authenticate
// the connecting account: wrong password, unknown account, an account that
// may not log in, or no host rule admitting it.
func IsLoginRejected(err error) bool {
	var my *mysql.MySQLError
	if errors.As(err, &my) {
		return my.Number == 1045 // ER_ACCESS_DENIED_ERROR
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		// invalid_authorization_specification, invalid_password
		return pg.Code == "28000" || pg.Code == "28P01"
	}
	return false
}

// ApplyConsoleProfile grants a Fleetdock-managed console role its access to
// exactly one database. Read roles get the readonly profile; write roles get
// readwrite. It is idempotent and safe to re-run (e.g. after new tables or
// schemas appear).
//
// PostgreSQL needs two extra steps:
//   - read roles default every transaction to read-only, so even a statement
//     the console misclassifies as a read cannot write;
//   - write roles join the database owner's role, because ALTER/DROP on
//     existing tables requires ownership — but only when the owner is an
//     ordinary role (no superuser, CREATEROLE, CREATEDB, REPLICATION or
//     BYPASSRLS) that owns no other database. Otherwise the console can still
//     write data and create new tables, but not alter tables it doesn't own.
func ApplyConsoleProfile(ctx context.Context, admin Admin, p ConnParams, user, host, database string, write bool) error {
	profile := ProfileReadonly
	if write {
		profile = ProfileReadWrite
	}
	if err := ApplyProfile(ctx, admin, p, user, host, database, profile); err != nil {
		return err
	}
	pg, ok := admin.(*Postgres)
	if !ok {
		return nil
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	role := quotePGIdent(user)
	if !write {
		_, err := conn.Exec(ctx, fmt.Sprintf("ALTER ROLE %s SET default_transaction_read_only = on", role))
		return err
	}
	// Delegate ownership only when it cannot reach beyond this database:
	// membership carries the owner's object ownership (all of it) and SET ROLE
	// carries its attributes, so an owner that is privileged or owns other
	// databases is never delegated.
	var owner string
	var delegable bool
	if err := conn.QueryRow(ctx, `
		SELECT r.rolname,
		       NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolreplication OR r.rolbypassrls)
		       AND (SELECT count(*) FROM pg_database o WHERE o.datdba = r.oid) = 1
		FROM pg_database d JOIN pg_roles r ON r.oid = d.datdba
		WHERE d.datname = $1`, database).Scan(&owner, &delegable); err != nil {
		return err
	}
	if owner == user {
		return nil
	}
	if !delegable {
		// The owner may have become broader since an earlier grant (e.g. it
		// now owns a second database): withdraw that delegation.
		var member bool
		if err := conn.QueryRow(ctx, `SELECT pg_has_role($1, $2, 'MEMBER')`, user, owner).Scan(&member); err == nil && member {
			_, err := conn.Exec(ctx, fmt.Sprintf("REVOKE %s FROM %s", quotePGIdent(owner), role))
			return err
		}
		return nil
	}
	_, err = conn.Exec(ctx, fmt.Sprintf("GRANT %s TO %s", quotePGIdent(owner), role))
	return err
}
