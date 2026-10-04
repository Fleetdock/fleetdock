package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Structure management: typed table and index DDL. The dashboard never sends
// SQL for these operations — it sends a spec, which is validated and rendered
// here (identifiers quoted, types from an allowlist, defaults either a quoted
// literal or an allowlisted expression). Free-form DDL stays in the console.

// ColumnSpec describes a column to create or redefine.
type ColumnSpec struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	// Default, when set, is a literal value (quoted for you) unless
	// DefaultIsExpression, in which case it must be one of the allowed
	// expressions (CURRENT_TIMESTAMP, NULL, TRUE, a number, ...).
	Default             *string `json:"default"`
	DefaultIsExpression bool    `json:"default_is_expression"`
	// AutoIncrement makes the column an auto-incrementing (MySQL) or identity
	// (PostgreSQL) column. Only valid for integer types.
	AutoIncrement bool   `json:"auto_increment"`
	Comment       string `json:"comment"`
	// KeepDefault (modify_column only) leaves the column's current default —
	// and on MySQL its AUTO_INCREMENT / ON UPDATE — exactly as it is, instead
	// of replacing it with Default. Defaults read back from a catalog cannot
	// be reliably re-entered as literals, so editing a column's type or
	// nullability should not force the user to.
	KeepDefault bool `json:"keep_default"`
}

// ForeignKeySpec describes a foreign key to create.
type ForeignKeySpec struct {
	Name       string   `json:"name"`
	Columns    []string `json:"columns"`
	RefTable   string   `json:"ref_table"`
	RefColumns []string `json:"ref_columns"`
	OnDelete   string   `json:"on_delete"`
	OnUpdate   string   `json:"on_update"`
}

// TableSpec describes a table to create.
type TableSpec struct {
	Name        string           `json:"name"`
	Columns     []ColumnSpec     `json:"columns"`
	PrimaryKey  []string         `json:"primary_key"`
	ForeignKeys []ForeignKeySpec `json:"foreign_keys"`
	Comment     string           `json:"comment"`
}

// AlterOpKind names an ALTER TABLE operation.
type AlterOpKind string

const (
	AlterAddColumn      AlterOpKind = "add_column"
	AlterDropColumn     AlterOpKind = "drop_column"
	AlterModifyColumn   AlterOpKind = "modify_column"
	AlterRenameColumn   AlterOpKind = "rename_column"
	AlterAddForeignKey  AlterOpKind = "add_foreign_key"
	AlterDropForeignKey AlterOpKind = "drop_foreign_key"
)

// AlterOp is one change to a table.
type AlterOp struct {
	Op AlterOpKind `json:"op"`
	// Name is the existing column (drop/modify/rename) or constraint
	// (drop_foreign_key).
	Name string `json:"name"`
	// NewName is the target name for rename_column.
	NewName    string          `json:"new_name"`
	Column     *ColumnSpec     `json:"column"`
	ForeignKey *ForeignKeySpec `json:"foreign_key"`
}

// IndexSpec describes an index to create.
type IndexSpec struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
}

// ForeignKey is an existing foreign key.
type ForeignKey struct {
	Name       string   `json:"name"`
	Columns    []string `json:"columns"`
	RefTable   string   `json:"ref_table"`
	RefColumns []string `json:"ref_columns"`
	OnDelete   string   `json:"on_delete"`
	OnUpdate   string   `json:"on_update"`
}

// ObjectKind is a non-table schema object kind.
type ObjectKind string

const (
	ObjectView              ObjectKind = "view"
	ObjectMaterializedView  ObjectKind = "materialized_view"
	ObjectFunction          ObjectKind = "function"
	ObjectProcedure         ObjectKind = "procedure"
	ObjectTrigger           ObjectKind = "trigger"
	ObjectSequence          ObjectKind = "sequence"
	ObjectEvent             ObjectKind = "event"
	maxObjectDefinitionSize            = 64 << 10
)

// DBObject is a view, routine, trigger, sequence or event.
type DBObject struct {
	Kind   ObjectKind `json:"kind"`
	Schema string     `json:"schema"`
	Name   string     `json:"name"`
	// Table is the table a trigger belongs to.
	Table      string  `json:"table,omitempty"`
	Definition *string `json:"definition"`
}

// Structure is the DDL surface of an engine.
type Structure interface {
	CreateTable(ctx context.Context, p ConnParams, database string, spec TableSpec) error
	AlterTable(ctx context.Context, p ConnParams, database, table string, ops []AlterOp) error
	DropTable(ctx context.Context, p ConnParams, database, table string) error
	TruncateTable(ctx context.Context, p ConnParams, database, table string) error
	RenameTable(ctx context.Context, p ConnParams, database, table, newName string) error
	CreateIndex(ctx context.Context, p ConnParams, database, table string, spec IndexSpec) error
	DropIndex(ctx context.Context, p ConnParams, database, table, index string) error
	ForeignKeys(ctx context.Context, p ConnParams, database, table string) ([]ForeignKey, error)
	Objects(ctx context.Context, p ConnParams, database string) ([]DBObject, error)
}

// StructureFor returns the DDL surface for the engine name.
func StructureFor(name string) (Structure, error) {
	c, err := For(name)
	if err != nil {
		return nil, err
	}
	s, ok := c.(Structure)
	if !ok {
		return nil, fmt.Errorf("engine: %s does not support structure editing", name)
	}
	return s, nil
}

// ---- validation shared by both engines ----

// objectNameRe accepts identifiers for new tables, columns, indexes and
// constraints: letters, digits, _ and $, not starting with a digit. Existing
// objects with exotic names can still be referenced (they are quoted), but new
// ones are kept portable.
var objectNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]{0,62}$`)

func validName(kind, name string) error {
	if !objectNameRe.MatchString(name) {
		return fmt.Errorf("invalid %s name %q: use letters, digits and _ (max 63 characters, not starting with a digit)", kind, name)
	}
	return nil
}

// existingName accepts any name a catalog could return, rejecting only what
// could break quoting.
func existingName(kind, name string) error {
	if name == "" || len(name) > 128 || strings.ContainsAny(name, "\x00\n\r") {
		return fmt.Errorf("invalid %s name", kind)
	}
	return nil
}

var (
	mysqlTypeRe = regexp.MustCompile(`(?i)^(tinyint|smallint|mediumint|int|integer|bigint|decimal|numeric|float|double|bit|bool|boolean|char|varchar|binary|varbinary|tinytext|text|mediumtext|longtext|tinyblob|blob|mediumblob|longblob|date|time|datetime|timestamp|year|json|geometry|point)(\(\d{1,5}(,\d{1,3})?\))?( unsigned)?( zerofill)?$`)
	pgTypeRe    = regexp.MustCompile(`(?i)^(smallint|integer|int|bigint|int2|int4|int8|decimal|numeric|real|double precision|float4|float8|money|boolean|bool|char|character|varchar|character varying|text|citext|bytea|date|time|timetz|time with time zone|time without time zone|timestamp|timestamptz|timestamp with time zone|timestamp without time zone|interval|uuid|json|jsonb|inet|cidr|macaddr|tsvector|xml|point)(\(\d{1,5}(,\d{1,3})?\))?(\[\])?$`)
	intTypeRe   = regexp.MustCompile(`(?i)^(tinyint|smallint|mediumint|int|integer|bigint|int2|int4|int8)\b`)
	numberRe    = regexp.MustCompile(`^-?\d{1,30}(\.\d{1,30})?$`)
	refActions  = map[string]string{"": "", "CASCADE": "CASCADE", "SET NULL": "SET NULL", "RESTRICT": "RESTRICT", "NO ACTION": "NO ACTION"}
)

// normalizeType lower-cases and collapses whitespace in a type name.
func normalizeType(t string) string {
	return strings.Join(strings.Fields(strings.ToLower(t)), " ")
}

func checkType(typeRe *regexp.Regexp, t string) (string, error) {
	n := normalizeType(t)
	n = strings.ReplaceAll(n, ", ", ",")
	n = strings.ReplaceAll(n, " (", "(")
	if !typeRe.MatchString(n) {
		return "", fmt.Errorf("unsupported column type %q (use the SQL console for exotic types)", t)
	}
	return n, nil
}

// defaultExprs are the default expressions accepted, per engine family.
var defaultExprs = map[string]bool{
	"NULL": true, "TRUE": true, "FALSE": true,
	"CURRENT_TIMESTAMP": true, "CURRENT_DATE": true, "CURRENT_TIME": true, "NOW()": true,
	"GEN_RANDOM_UUID()": true, "UUID()": true, "LOCALTIMESTAMP": true,
}

// renderDefault renders a column default, quoting literals with quote.
func renderDefault(c ColumnSpec, quote func(string) string) (string, error) {
	if c.Default == nil {
		return "", nil
	}
	v := strings.TrimSpace(*c.Default)
	if !c.DefaultIsExpression {
		return " DEFAULT " + quote(*c.Default), nil
	}
	if numberRe.MatchString(v) || defaultExprs[strings.ToUpper(v)] {
		return " DEFAULT " + v, nil
	}
	return "", fmt.Errorf("default expression %q is not allowed; use a literal value, a number, NULL, TRUE/FALSE, CURRENT_TIMESTAMP or NOW()", v)
}

func refAction(kind, a string) (string, error) {
	v, ok := refActions[strings.ToUpper(strings.TrimSpace(a))]
	if !ok {
		return "", fmt.Errorf("invalid %s action %q (CASCADE, SET NULL, RESTRICT, NO ACTION)", kind, a)
	}
	return v, nil
}

// renderFK renders a FOREIGN KEY clause. quoteTable renders the referenced
// table (which the caller has validated).
func renderFK(fk ForeignKeySpec, quote, quoteTable func(string) string) (string, error) {
	if len(fk.Columns) == 0 || len(fk.Columns) != len(fk.RefColumns) {
		return "", fmt.Errorf("a foreign key needs the same number of columns on both sides")
	}
	if err := existingName("table", fk.RefTable); err != nil {
		return "", err
	}
	cols := make([]string, len(fk.Columns))
	refs := make([]string, len(fk.RefColumns))
	for i := range fk.Columns {
		if err := existingName("column", fk.Columns[i]); err != nil {
			return "", err
		}
		if err := existingName("column", fk.RefColumns[i]); err != nil {
			return "", err
		}
		cols[i], refs[i] = quote(fk.Columns[i]), quote(fk.RefColumns[i])
	}
	var b strings.Builder
	if fk.Name != "" {
		if err := validName("constraint", fk.Name); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "CONSTRAINT %s ", quote(fk.Name))
	}
	fmt.Fprintf(&b, "FOREIGN KEY (%s) REFERENCES %s (%s)", strings.Join(cols, ", "), quoteTable(fk.RefTable), strings.Join(refs, ", "))
	onDel, err := refAction("ON DELETE", fk.OnDelete)
	if err != nil {
		return "", err
	}
	onUpd, err := refAction("ON UPDATE", fk.OnUpdate)
	if err != nil {
		return "", err
	}
	if onDel != "" {
		b.WriteString(" ON DELETE " + onDel)
	}
	if onUpd != "" {
		b.WriteString(" ON UPDATE " + onUpd)
	}
	return b.String(), nil
}

func quotedList(names []string, quote func(string) string) ([]string, error) {
	out := make([]string, len(names))
	for i, n := range names {
		if err := existingName("column", n); err != nil {
			return nil, err
		}
		out[i] = quote(n)
	}
	return out, nil
}

func truncateDefinition(s *string) *string {
	if s == nil || len(*s) <= maxObjectDefinitionSize {
		return s
	}
	t := (*s)[:maxObjectDefinitionSize] + "\n-- (truncated)"
	return &t
}
