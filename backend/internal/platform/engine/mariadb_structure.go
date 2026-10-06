package engine

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func mysqlLiteral(s string) string { return "'" + escapeMySQLString(s) + "'" }

// keptDefault is a column's current default clause as read from the
// server's own catalog (trusted text, not user input).
type keptDefault struct {
	clause string // " DEFAULT ..." or ""
	extra  string // " AUTO_INCREMENT", " ON UPDATE ...", or ""
}

// currentDefault reads a column's default and extra attributes so MODIFY can
// carry them over. MariaDB reports defaults as SQL expressions ('abc',
// current_timestamp(), NULL); MySQL 8 reports literals bare and marks
// expression defaults DEFAULT_GENERATED.
func (m *MariaDB) currentDefault(ctx context.Context, q queryer, database, table, column string) (keptDefault, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT COLUMN_DEFAULT, EXTRA, IS_NULLABLE, @@version LIKE '%MariaDB%'
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME = ?`, database, table, column)
	if err != nil {
		return keptDefault{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return keptDefault{}, err
		}
		return keptDefault{}, fmt.Errorf("column %q not found", column)
	}
	var def sql.NullString
	var extra, nullable string
	var mariadb bool
	if err := rows.Scan(&def, &extra, &nullable, &mariadb); err != nil {
		return keptDefault{}, err
	}
	var k keptDefault
	lowerExtra := strings.ToLower(extra)
	switch {
	case !def.Valid:
	case mariadb:
		k.clause = " DEFAULT " + def.String
	case strings.Contains(lowerExtra, "default_generated"):
		if strings.HasPrefix(strings.ToUpper(def.String), "CURRENT_TIMESTAMP") {
			k.clause = " DEFAULT " + def.String
		} else {
			k.clause = " DEFAULT (" + def.String + ")"
		}
	default:
		k.clause = " DEFAULT " + mysqlLiteral(def.String)
	}
	if strings.Contains(lowerExtra, "auto_increment") {
		k.extra += " AUTO_INCREMENT"
	}
	if i := strings.Index(lowerExtra, "on update "); i >= 0 {
		k.extra += " " + strings.TrimSpace(strings.ReplaceAll(extra[i:], "DEFAULT_GENERATED", ""))
	}
	return k, nil
}

func mysqlColumnDef(c ColumnSpec) (string, error) {
	return mysqlColumnDefKeep(c, nil)
}

func mysqlColumnDefKeep(c ColumnSpec, keep *keptDefault) (string, error) {
	if err := validName("column", c.Name); err != nil {
		return "", err
	}
	typ, err := checkType(mysqlTypeRe, c.Type)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", quoteMySQLIdent(c.Name), typ)
	if c.Nullable {
		b.WriteString(" NULL")
	} else {
		b.WriteString(" NOT NULL")
	}
	if keep != nil {
		b.WriteString(keep.clause + keep.extra)
		if c.Comment != "" {
			b.WriteString(" COMMENT " + mysqlLiteral(c.Comment))
		}
		return b.String(), nil
	}
	def, err := renderDefault(c, mysqlLiteral)
	if err != nil {
		return "", err
	}
	b.WriteString(def)
	if c.AutoIncrement {
		if !intTypeRe.MatchString(typ) {
			return "", fmt.Errorf("auto increment needs an integer column, not %s", typ)
		}
		b.WriteString(" AUTO_INCREMENT")
	}
	if c.Comment != "" {
		b.WriteString(" COMMENT " + mysqlLiteral(c.Comment))
	}
	return b.String(), nil
}

func (m *MariaDB) qualified(database, table string) (string, error) {
	if !identRe.MatchString(database) {
		return "", fmt.Errorf("invalid database name")
	}
	if !validTableName(table) {
		return "", fmt.Errorf("invalid table name")
	}
	return quoteMySQLIdent(database) + "." + quoteMySQLIdent(table), nil
}

// ddl runs DDL statements on one session. MySQL commits each DDL statement
// implicitly, which is why AlterTable folds everything into one statement.
func (m *MariaDB) ddl(ctx context.Context, p ConnParams, stmts ...string) error {
	conn, closeSession, err := m.session(ctx, p)
	if err != nil {
		return err
	}
	defer closeSession()
	for _, s := range stmts {
		if _, err := conn.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (m *MariaDB) currentDefaultFor(ctx context.Context, p ConnParams, database, table, column string) (keptDefault, error) {
	db, err := m.open(p)
	if err != nil {
		return keptDefault{}, err
	}
	defer db.Close()
	return m.currentDefault(ctx, db, database, table, column)
}

// CreateTable creates a table from a spec.
func (m *MariaDB) CreateTable(ctx context.Context, p ConnParams, database string, spec TableSpec) error {
	if err := validName("table", spec.Name); err != nil {
		return err
	}
	q, err := m.qualified(database, spec.Name)
	if err != nil {
		return err
	}
	if len(spec.Columns) == 0 {
		return fmt.Errorf("a table needs at least one column")
	}
	var parts []string
	for _, c := range spec.Columns {
		def, err := mysqlColumnDef(c)
		if err != nil {
			return err
		}
		parts = append(parts, def)
	}
	if len(spec.PrimaryKey) > 0 {
		cols, err := quotedList(spec.PrimaryKey, quoteMySQLIdent)
		if err != nil {
			return err
		}
		parts = append(parts, "PRIMARY KEY ("+strings.Join(cols, ", ")+")")
	}
	refTable := func(t string) string { return quoteMySQLIdent(database) + "." + quoteMySQLIdent(t) }
	for _, fk := range spec.ForeignKeys {
		clause, err := renderFK(fk, quoteMySQLIdent, refTable)
		if err != nil {
			return err
		}
		parts = append(parts, clause)
	}
	stmt := fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", q, strings.Join(parts, ",\n  "))
	if spec.Comment != "" {
		stmt += " COMMENT=" + mysqlLiteral(spec.Comment)
	}
	return m.ddl(ctx, p, stmt)
}

// AlterTable applies all ops in a single ALTER TABLE statement, so they
// succeed or fail together.
func (m *MariaDB) AlterTable(ctx context.Context, p ConnParams, database, table string, ops []AlterOp) error {
	q, err := m.qualified(database, table)
	if err != nil {
		return err
	}
	if len(ops) == 0 {
		return fmt.Errorf("no changes")
	}
	refTable := func(t string) string { return quoteMySQLIdent(database) + "." + quoteMySQLIdent(t) }
	// All clauses of one ALTER TABLE see the table as it was before the
	// statement, so a clause cannot refer to a column an earlier clause adds
	// or renames. Say so plainly instead of surfacing "unknown column".
	introduced := map[string]bool{}
	var clauses []string
	for _, op := range ops {
		if introduced[op.Name] {
			return fmt.Errorf("column %q is created or renamed by this same change; save first, then edit it", op.Name)
		}
		switch op.Op {
		case AlterAddColumn:
			if op.Column != nil {
				introduced[op.Column.Name] = true
			}
		case AlterRenameColumn:
			introduced[op.NewName] = true
		case AlterModifyColumn:
			if op.Column != nil && op.Column.Name != op.Name {
				introduced[op.Column.Name] = true
			}
		}
		switch op.Op {
		case AlterAddColumn:
			if op.Column == nil {
				return fmt.Errorf("add_column needs a column")
			}
			def, err := mysqlColumnDef(*op.Column)
			if err != nil {
				return err
			}
			clauses = append(clauses, "ADD COLUMN "+def)
		case AlterDropColumn:
			if err := existingName("column", op.Name); err != nil {
				return err
			}
			clauses = append(clauses, "DROP COLUMN "+quoteMySQLIdent(op.Name))
		case AlterModifyColumn:
			if op.Column == nil {
				return fmt.Errorf("modify_column needs a column")
			}
			if err := existingName("column", op.Name); err != nil {
				return err
			}
			var keep *keptDefault
			if op.Column.KeepDefault {
				k, err := m.currentDefaultFor(ctx, p, database, table, op.Name)
				if err != nil {
					return err
				}
				keep = &k
			}
			def, err := mysqlColumnDefKeep(*op.Column, keep)
			if err != nil {
				return err
			}
			// CHANGE both renames and redefines; with the same name it is
			// MODIFY, and works on every MySQL/MariaDB version.
			clauses = append(clauses, "CHANGE COLUMN "+quoteMySQLIdent(op.Name)+" "+def)
		case AlterRenameColumn:
			if err := existingName("column", op.Name); err != nil {
				return err
			}
			if err := validName("column", op.NewName); err != nil {
				return err
			}
			clauses = append(clauses, "RENAME COLUMN "+quoteMySQLIdent(op.Name)+" TO "+quoteMySQLIdent(op.NewName))
		case AlterAddForeignKey:
			if op.ForeignKey == nil {
				return fmt.Errorf("add_foreign_key needs a foreign key")
			}
			clause, err := renderFK(*op.ForeignKey, quoteMySQLIdent, refTable)
			if err != nil {
				return err
			}
			clauses = append(clauses, "ADD "+clause)
		case AlterDropForeignKey:
			if err := existingName("constraint", op.Name); err != nil {
				return err
			}
			clauses = append(clauses, "DROP FOREIGN KEY "+quoteMySQLIdent(op.Name))
		default:
			return fmt.Errorf("unsupported alter operation %q", op.Op)
		}
	}
	return m.ddl(ctx, p, "ALTER TABLE "+q+"\n  "+strings.Join(clauses, ",\n  "))
}

// DropTable drops a table.
func (m *MariaDB) DropTable(ctx context.Context, p ConnParams, database, table string) error {
	q, err := m.qualified(database, table)
	if err != nil {
		return err
	}
	return m.ddl(ctx, p, "DROP TABLE "+q)
}

// TruncateTable removes every row.
func (m *MariaDB) TruncateTable(ctx context.Context, p ConnParams, database, table string) error {
	q, err := m.qualified(database, table)
	if err != nil {
		return err
	}
	return m.ddl(ctx, p, "TRUNCATE TABLE "+q)
}

// RenameTable renames a table within its database.
func (m *MariaDB) RenameTable(ctx context.Context, p ConnParams, database, table, newName string) error {
	q, err := m.qualified(database, table)
	if err != nil {
		return err
	}
	if err := validName("table", newName); err != nil {
		return err
	}
	return m.ddl(ctx, p, fmt.Sprintf("RENAME TABLE %s TO %s.%s", q, quoteMySQLIdent(database), quoteMySQLIdent(newName)))
}

// CreateIndex creates an index.
func (m *MariaDB) CreateIndex(ctx context.Context, p ConnParams, database, table string, spec IndexSpec) error {
	q, err := m.qualified(database, table)
	if err != nil {
		return err
	}
	if err := validName("index", spec.Name); err != nil {
		return err
	}
	if len(spec.Columns) == 0 {
		return fmt.Errorf("an index needs at least one column")
	}
	cols, err := quotedList(spec.Columns, quoteMySQLIdent)
	if err != nil {
		return err
	}
	unique := ""
	if spec.Unique {
		unique = "UNIQUE "
	}
	return m.ddl(ctx, p, fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", unique, quoteMySQLIdent(spec.Name), q, strings.Join(cols, ", ")))
}

// DropIndex drops an index (PRIMARY drops the primary key).
func (m *MariaDB) DropIndex(ctx context.Context, p ConnParams, database, table, index string) error {
	q, err := m.qualified(database, table)
	if err != nil {
		return err
	}
	if err := existingName("index", index); err != nil {
		return err
	}
	if strings.EqualFold(index, "PRIMARY") {
		return m.ddl(ctx, p, "ALTER TABLE "+q+" DROP PRIMARY KEY")
	}
	return m.ddl(ctx, p, fmt.Sprintf("DROP INDEX %s ON %s", quoteMySQLIdent(index), q))
}

// ForeignKeys lists a table's outgoing foreign keys.
func (m *MariaDB) ForeignKeys(ctx context.Context, p ConnParams, database, table string) ([]ForeignKey, error) {
	if _, err := m.qualified(database, table); err != nil {
		return nil, err
	}
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT k.CONSTRAINT_NAME, k.COLUMN_NAME, k.REFERENCED_TABLE_NAME, k.REFERENCED_COLUMN_NAME,
		       r.DELETE_RULE, r.UPDATE_RULE
		FROM information_schema.KEY_COLUMN_USAGE k
		JOIN information_schema.REFERENTIAL_CONSTRAINTS r
		  ON r.CONSTRAINT_SCHEMA = k.CONSTRAINT_SCHEMA AND r.CONSTRAINT_NAME = k.CONSTRAINT_NAME
		 AND r.TABLE_NAME = k.TABLE_NAME
		WHERE k.TABLE_SCHEMA = ? AND k.TABLE_NAME = ? AND k.REFERENCED_TABLE_NAME IS NOT NULL
		ORDER BY k.CONSTRAINT_NAME, k.ORDINAL_POSITION`, database, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ForeignKey{}
	idx := map[string]int{}
	for rows.Next() {
		var name, col, refTable, refCol, onDel, onUpd string
		if err := rows.Scan(&name, &col, &refTable, &refCol, &onDel, &onUpd); err != nil {
			return nil, err
		}
		i, ok := idx[name]
		if !ok {
			out = append(out, ForeignKey{Name: name, RefTable: refTable, OnDelete: onDel, OnUpdate: onUpd})
			i = len(out) - 1
			idx[name] = i
		}
		out[i].Columns = append(out[i].Columns, col)
		out[i].RefColumns = append(out[i].RefColumns, refCol)
	}
	return out, rows.Err()
}

// Objects lists views, routines, triggers, events and (MariaDB) sequences.
func (m *MariaDB) Objects(ctx context.Context, p ConnParams, database string) ([]DBObject, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	out := []DBObject{}
	add := func(kind ObjectKind, q string) error {
		rows, err := db.QueryContext(ctx, q, database)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			o := DBObject{Kind: kind, Schema: database}
			var table, def, typ sql.NullString
			if err := rows.Scan(&o.Name, &table, &def, &typ); err != nil {
				return err
			}
			if typ.Valid && strings.EqualFold(typ.String, "PROCEDURE") {
				o.Kind = ObjectProcedure
			}
			o.Table = table.String
			if def.Valid {
				o.Definition = truncateDefinition(&def.String)
			}
			out = append(out, o)
		}
		return rows.Err()
	}
	queries := []struct {
		kind ObjectKind
		q    string
	}{
		{ObjectView, `SELECT TABLE_NAME, NULL, VIEW_DEFINITION, NULL FROM information_schema.VIEWS WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME`},
		{ObjectFunction, `SELECT ROUTINE_NAME, NULL, ROUTINE_DEFINITION, ROUTINE_TYPE FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA = ? ORDER BY ROUTINE_TYPE, ROUTINE_NAME`},
		{ObjectTrigger, `SELECT TRIGGER_NAME, EVENT_OBJECT_TABLE, CONCAT(ACTION_TIMING, ' ', EVENT_MANIPULATION, ': ', ACTION_STATEMENT), NULL FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = ? ORDER BY TRIGGER_NAME`},
		{ObjectEvent, `SELECT EVENT_NAME, NULL, EVENT_DEFINITION, NULL FROM information_schema.EVENTS WHERE EVENT_SCHEMA = ? ORDER BY EVENT_NAME`},
	}
	for _, qq := range queries {
		if err := add(qq.kind, qq.q); err != nil {
			return nil, err
		}
	}
	// Sequences exist on MariaDB 10.3+ only; MySQL reports no such table type.
	_ = add(ObjectSequence, `SELECT TABLE_NAME, NULL, NULL, NULL FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'SEQUENCE' ORDER BY TABLE_NAME`)
	return out, nil
}
