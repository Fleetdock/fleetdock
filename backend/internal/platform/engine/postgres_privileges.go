package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

var _ SchemaPrivileges = (*Postgres)(nil)

// SchemaAccess reads a schema's owner, grants and default privileges.
func (pg *Postgres) SchemaAccess(ctx context.Context, p ConnParams, database, schema string) (*SchemaAccess, error) {
	if !validDBName(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	if err := existingName("schema", schema); err != nil {
		return nil, err
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	out := &SchemaAccess{Schema: schema, Grants: []SchemaRoleGrant{}, Defaults: []DefaultPrivilege{}, Roles: []string{}}
	err = conn.QueryRow(ctx, `SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname = $1`, schema).Scan(&out.Owner)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("schema %s does not exist", schema)
	}
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(ctx, `
		SELECT CASE WHEN a.grantee = 0 THEN 'PUBLIC' ELSE pg_get_userbyid(a.grantee) END, a.privilege_type
		FROM pg_namespace n, aclexplode(n.nspacl) a
		WHERE n.nspname = $1 AND a.grantee <> n.nspowner
		ORDER BY 1`, schema)
	if err != nil {
		return nil, err
	}
	byRole := map[string]*SchemaRoleGrant{}
	var order []string
	for rows.Next() {
		var role, priv string
		if err := rows.Scan(&role, &priv); err != nil {
			rows.Close()
			return nil, err
		}
		g := byRole[role]
		if g == nil {
			g = &SchemaRoleGrant{Role: role}
			byRole[role] = g
			order = append(order, role)
		}
		switch priv {
		case "USAGE":
			g.Usage = true
		case "CREATE":
			g.Create = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, r := range order {
		out.Grants = append(out.Grants, *byRole[r])
	}

	rows, err = conn.Query(ctx, `
		SELECT pg_get_userbyid(d.defaclrole), d.defaclobjtype::text,
		       CASE WHEN a.grantee = 0 THEN 'PUBLIC' ELSE pg_get_userbyid(a.grantee) END, a.privilege_type
		FROM pg_default_acl d
		JOIN pg_namespace n ON n.oid = d.defaclnamespace,
		     aclexplode(d.defaclacl) a
		WHERE n.nspname = $1
		ORDER BY 1, 2, 3`, schema)
	if err != nil {
		return nil, err
	}
	codes := map[string]string{}
	for name, k := range defaultPrivilegeKinds {
		codes[k.code] = name
	}
	type key struct{ creator, kind, grantee string }
	defaults := map[key][]string{}
	var keys []key
	for rows.Next() {
		var creator, code, grantee, priv string
		if err := rows.Scan(&creator, &code, &grantee, &priv); err != nil {
			rows.Close()
			return nil, err
		}
		kind, ok := codes[code]
		if !ok {
			kind = code // schemas ("n") cannot be set per schema; keep anything else visible
		}
		k := key{creator, kind, grantee}
		if _, seen := defaults[k]; !seen {
			keys = append(keys, k)
		}
		defaults[k] = append(defaults[k], priv)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, k := range keys {
		privs := defaults[k]
		sort.Strings(privs)
		out.Defaults = append(out.Defaults, DefaultPrivilege{Creator: k.creator, ObjectType: k.kind, Grantee: k.grantee, Privileges: privs})
	}

	rows, err = conn.Query(ctx, `SELECT rolname FROM pg_roles WHERE rolname !~ '^pg_' ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out.Roles = append(out.Roles, r)
	}
	return out, rows.Err()
}

// checkGrantee validates a grantee: PUBLIC or an existing role.
func checkGrantee(ctx context.Context, conn *pgx.Conn, role string) error {
	if strings.EqualFold(role, "public") {
		return nil
	}
	if err := existingName("role", role); err != nil {
		return err
	}
	ok, err := roleExists(ctx, conn, role)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("role %q does not exist", role)
	}
	return nil
}

// SetSchemaGrant leaves role with exactly the USAGE and CREATE asked for.
func (pg *Postgres) SetSchemaGrant(ctx context.Context, p ConnParams, database, schema, role string, usage, create bool) error {
	if err := checkUserSchema(schema); err != nil {
		return err
	}
	return pg.ddlTx(ctx, p, database, func(ctx context.Context, conn *pgx.Conn) ([]string, error) {
		var owner string
		err := conn.QueryRow(ctx, `SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname = $1`, schema).Scan(&owner)
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("schema %s does not exist", schema)
		}
		if err != nil {
			return nil, err
		}
		if err := checkGrantee(ctx, conn, role); err != nil {
			return nil, err
		}
		if role == owner {
			return nil, fmt.Errorf("%s owns %s and always has every privilege on it", role, schema)
		}
		s, r := quotePGIdent(schema), granteeSQL(role)
		var stmts []string
		for _, pr := range []struct {
			name string
			on   bool
		}{{"USAGE", usage}, {"CREATE", create}} {
			if pr.on {
				stmts = append(stmts, fmt.Sprintf("GRANT %s ON SCHEMA %s TO %s", pr.name, s, r))
			} else {
				stmts = append(stmts, fmt.Sprintf("REVOKE %s ON SCHEMA %s FROM %s", pr.name, s, r))
			}
		}
		return stmts, nil
	})
}

// AlterDefaultPrivileges runs one ALTER DEFAULT PRIVILEGES … IN SCHEMA.
func (pg *Postgres) AlterDefaultPrivileges(ctx context.Context, p ConnParams, database, schema string, c DefaultPrivilegeChange) error {
	if err := checkUserSchema(schema); err != nil {
		return err
	}
	privs, err := checkDefaultPrivileges(c.ObjectType, c.Privileges)
	if err != nil {
		return err
	}
	return pg.ddlTx(ctx, p, database, func(ctx context.Context, conn *pgx.Conn) ([]string, error) {
		if ok, err := schemaExists(ctx, conn, schema); err != nil {
			return nil, err
		} else if !ok {
			return nil, fmt.Errorf("schema %s does not exist", schema)
		}
		if err := checkGrantee(ctx, conn, c.Grantee); err != nil {
			return nil, err
		}
		stmt := "ALTER DEFAULT PRIVILEGES "
		if c.Creator != "" {
			if err := checkOwner(ctx, conn, c.Creator); err != nil {
				return nil, err
			}
			stmt += "FOR ROLE " + quotePGIdent(c.Creator) + " "
		}
		stmt += "IN SCHEMA " + quotePGIdent(schema) + " "
		on := strings.ToUpper(c.ObjectType)
		if c.Revoke {
			stmt += fmt.Sprintf("REVOKE %s ON %s FROM %s", strings.Join(privs, ", "), on, granteeSQL(c.Grantee))
		} else {
			stmt += fmt.Sprintf("GRANT %s ON %s TO %s", strings.Join(privs, ", "), on, granteeSQL(c.Grantee))
		}
		return []string{stmt}, nil
	})
}

// SearchPaths reads the search_path settings of the database and its roles.
func (pg *Postgres) SearchPaths(ctx context.Context, p ConnParams, database string) (*SearchPaths, error) {
	if !validDBName(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	out := &SearchPaths{Roles: []RoleSearchPath{}}
	if err := conn.QueryRow(ctx, `SELECT boot_val FROM pg_settings WHERE name = 'search_path'`).Scan(&out.Server); err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, `
		SELECT coalesce(r.rolname, ''), s.setconfig
		FROM pg_db_role_setting s
		LEFT JOIN pg_roles r ON r.oid = s.setrole
		WHERE s.setdatabase = (SELECT oid FROM pg_database WHERE datname = current_database())
		ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		var config []string
		if err := rows.Scan(&role, &config); err != nil {
			return nil, err
		}
		for _, c := range config {
			path, ok := strings.CutPrefix(c, "search_path=")
			if !ok {
				continue
			}
			if role == "" {
				out.Database = path
			} else {
				out.Roles = append(out.Roles, RoleSearchPath{Role: role, Path: path})
			}
		}
	}
	return out, rows.Err()
}

// SetSearchPath sets or resets the search_path of the database or of a role
// in it.
func (pg *Postgres) SetSearchPath(ctx context.Context, p ConnParams, database, role string, schemas []string) error {
	if !validDBName(database) {
		return fmt.Errorf("invalid database name")
	}
	value := "RESET search_path"
	if len(schemas) > 0 {
		list, err := searchPathList(schemas)
		if err != nil {
			return err
		}
		value = "SET search_path TO " + list
	}
	return pg.ddlTx(ctx, p, database, func(ctx context.Context, conn *pgx.Conn) ([]string, error) {
		if role == "" {
			return []string{"ALTER DATABASE " + quotePGIdent(database) + " " + value}, nil
		}
		if err := checkOwner(ctx, conn, role); err != nil {
			return nil, err
		}
		return []string{"ALTER ROLE " + quotePGIdent(role) + " IN DATABASE " + quotePGIdent(database) + " " + value}, nil
	})
}
