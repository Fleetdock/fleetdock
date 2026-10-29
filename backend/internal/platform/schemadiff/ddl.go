package schemadiff

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Script is the DDL that turns one schema into another.
type Script struct {
	SQL     string   `json:"sql"`
	Changes []Change `json:"changes"`
	// Review counts the steps left commented out (dropping a table or a
	// column loses data) or that need a manual change.
	Review int `json:"review"`
}

// Migrate writes the DDL that makes a database whose structure is from look
// like to. Destructive steps (dropping tables and columns) are written
// commented out with "-- REVIEW:"; what cannot be generated is described in
// a "-- manual:" comment.
func Migrate(from, to *Schema) Script {
	changes := Diff(from, to)
	w := &writer{}
	if to.Dialect == Postgres {
		pgScript(w, from, to, changes)
	} else {
		mysqlScript(w, from, to, changes)
	}
	return Script{SQL: w.String(), Changes: changes, Review: w.review}
}

type writer struct {
	b      strings.Builder
	review int
}

func (w *writer) stmt(format string, args ...any) {
	fmt.Fprintf(&w.b, format, args...)
	w.b.WriteString(";\n")
}

// destructive writes a statement commented out.
func (w *writer) destructive(why, format string, args ...any) {
	w.review++
	w.b.WriteString("-- REVIEW (" + why + "): " + fmt.Sprintf(format, args...) + ";\n")
}

func (w *writer) manual(format string, args ...any) {
	w.review++
	w.b.WriteString("-- manual: " + fmt.Sprintf(format, args...) + "\n")
}

func (w *writer) raw(s string) { w.b.WriteString(s) }

func (w *writer) section(title string) {
	if w.b.Len() > 0 {
		w.b.WriteString("\n")
	}
	w.b.WriteString("-- " + title + "\n")
}

func (w *writer) String() string { return w.b.String() }

// changesOf returns the changes of a kind and op ("" = any op).
func changesOf(changes []Change, kind string, ops ...string) []Change {
	var out []Change
	for _, c := range changes {
		if c.Kind != kind {
			continue
		}
		if len(ops) == 0 {
			out = append(out, c)
			continue
		}
		for _, op := range ops {
			if c.Op == op {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

func findTable(s *Schema, schema, name string) *Table {
	for i := range s.Tables {
		if s.Tables[i].Schema == schema && s.Tables[i].Name == name {
			return &s.Tables[i]
		}
	}
	return nil
}

func findObject(l []Object, schema, table, name string) *Object {
	for i := range l {
		if l[i].Schema == schema && l[i].Table == table && l[i].Name == name {
			return &l[i]
		}
	}
	return nil
}

func findColumn(t *Table, name string) *Column {
	if t == nil {
		return nil
	}
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i]
		}
	}
	return nil
}

// ---- MySQL / MariaDB ----

// QuoteMySQL quotes a MySQL identifier.
func QuoteMySQL(n string) string { return "`" + strings.ReplaceAll(n, "`", "``") + "`" }

// QuotePG quotes a PostgreSQL identifier.
func QuotePG(n string) string { return `"` + strings.ReplaceAll(n, `"`, `""`) + `"` }

func pgTable(schema, name string) string {
	if schema == "" {
		return QuotePG(name)
	}
	return QuotePG(schema) + "." + QuotePG(name)
}

// CreateTable renders a table's CREATE TABLE (for display; PostgreSQL
// constraints are shown inline and indexes after it).
func CreateTable(dialect string, t *Table) string {
	if dialect == Postgres {
		var b strings.Builder
		b.WriteString(pgCreateTable(t))
		parts := []string{}
		for _, c := range t.Constraints {
			parts = append(parts, "ALTER TABLE "+pgTable(t.Schema, t.Name)+" ADD CONSTRAINT "+QuotePG(c.Name)+" "+c.Def+";")
		}
		for _, ix := range t.Indexes {
			parts = append(parts, ix.Def+";")
		}
		if len(parts) > 0 {
			b.WriteString(";\n" + strings.Join(parts, "\n"))
		}
		return b.String()
	}
	return mysqlCreateTable(t)
}

func mysqlCreateTable(t *Table) string {
	var lines []string
	for _, c := range t.Columns {
		lines = append(lines, c.Def)
	}
	idx := append([]Object(nil), t.Indexes...)
	sort.SliceStable(idx, func(i, j int) bool { return idx[i].Kind == "PRIMARY" && idx[j].Kind != "PRIMARY" })
	for _, ix := range idx {
		lines = append(lines, ix.Def)
	}
	for _, c := range t.Constraints {
		lines = append(lines, c.Def)
	}
	s := "CREATE TABLE " + QuoteMySQL(t.Name) + " (\n  " + strings.Join(lines, ",\n  ") + "\n)"
	if t.Options != "" {
		s += " " + t.Options
	}
	return s
}

func mysqlDelimiter(body string) string {
	for _, d := range []string{"$$", "//", ";;", "$$$"} {
		if !strings.Contains(body, d) {
			return d
		}
	}
	return "$$$$"
}

func mysqlCompound(w *writer, def string) {
	d := mysqlDelimiter(def)
	w.raw("DELIMITER " + d + "\n" + def + " " + d + "\nDELIMITER ;\n")
}

func mysqlDropIndex(ix Object) string {
	if ix.Kind == "PRIMARY" {
		return "DROP PRIMARY KEY"
	}
	return "DROP INDEX " + QuoteMySQL(ix.Name)
}

func mysqlDropConstraint(engine string, c Object) string {
	switch {
	case c.Kind == "FOREIGN":
		return "DROP FOREIGN KEY " + QuoteMySQL(c.Name)
	case engine == "mariadb":
		return "DROP CONSTRAINT " + QuoteMySQL(c.Name)
	default:
		return "DROP CHECK " + QuoteMySQL(c.Name)
	}
}

func mysqlScript(w *writer, from, to *Schema, changes []Change) {
	tableWork := false
	for _, c := range changes {
		switch c.Kind {
		case KindTable, KindColumn, KindIndex, KindConstraint, KindOptions:
			tableWork = true
		}
	}
	if tableWork {
		w.stmt("SET FOREIGN_KEY_CHECKS = 0")
	}
	// Drops of dependent objects first.
	if l := changesOf(changes, KindTrigger, Removed, Changed); len(l) > 0 {
		w.section("Triggers to drop or replace")
		for _, c := range l {
			w.stmt("DROP TRIGGER IF EXISTS %s", QuoteMySQL(c.Name))
		}
	}
	if l := changesOf(changes, KindView, Removed); len(l) > 0 {
		w.section("Views to drop")
		for _, c := range l {
			w.stmt("DROP VIEW IF EXISTS %s", QuoteMySQL(c.Name))
		}
	}
	if l := changesOf(changes, KindRoutine, Removed, Changed); len(l) > 0 {
		w.section("Routines to drop or replace")
		for _, c := range l {
			o := findObject(from.Routines, c.Schema, "", c.Name)
			kind := "PROCEDURE"
			if o != nil && o.Kind != "" {
				kind = o.Kind
			}
			w.stmt("DROP %s IF EXISTS %s", kind, QuoteMySQL(c.Name))
		}
	}
	if l := changesOf(changes, KindConstraint, Removed, Changed); len(l) > 0 {
		w.section("Constraints to drop or replace")
		for _, c := range l {
			o := findObject(findTableObjects(from, c, true), "", "", c.Name)
			if o == nil {
				continue
			}
			w.stmt("ALTER TABLE %s %s", QuoteMySQL(c.Table), mysqlDropConstraint(from.Engine, *o))
		}
	}
	if l := changesOf(changes, KindTable, Added); len(l) > 0 {
		w.section("New tables")
		for _, c := range l {
			w.stmt("%s", mysqlCreateTable(findTable(to, c.Schema, c.Table)))
		}
	}
	// Changed tables, one table at a time.
	for _, name := range changedTables(changes) {
		t := findTable(to, "", name)
		old := findTable(from, "", name)
		w.section("Table " + name)
		tq := QuoteMySQL(name)
		for _, c := range changes {
			if c.Table != name || c.Kind == KindTable {
				continue
			}
			switch {
			case c.Kind == KindOptions:
				if strings.Contains(strings.ToUpper(c.After+c.Before), "PARTITION") {
					w.manual("table options of %s changed (partitioning): %s", tq, c.After)
					continue
				}
				w.stmt("ALTER TABLE %s %s", tq, c.After)
			case c.Kind == KindIndex && c.Op != Added:
				o := findObject(old.Indexes, "", "", c.Name)
				w.stmt("ALTER TABLE %s %s", tq, mysqlDropIndex(*o))
			}
		}
		for i, col := range t.Columns {
			ch := findChange(changes, KindColumn, name, col.Name)
			if ch == nil {
				continue
			}
			switch ch.Op {
			case Added:
				pos := " FIRST"
				if i > 0 {
					pos = " AFTER " + QuoteMySQL(t.Columns[i-1].Name)
				}
				w.stmt("ALTER TABLE %s ADD COLUMN %s%s", tq, col.Def, pos)
			case Changed:
				w.stmt("ALTER TABLE %s MODIFY COLUMN %s", tq, col.Def)
			}
		}
		for _, c := range changesOf(changes, KindColumn, Removed) {
			if c.Table == name {
				w.destructive("drops the column's data", "ALTER TABLE %s DROP COLUMN %s", tq, QuoteMySQL(c.Name))
			}
		}
		for _, c := range changesOf(changes, KindIndex, Added, Changed) {
			if c.Table == name {
				w.stmt("ALTER TABLE %s ADD %s", tq, c.After)
			}
		}
	}
	if l := changesOf(changes, KindConstraint, Added, Changed); len(l) > 0 {
		w.section("Constraints to add")
		for _, c := range l {
			w.stmt("ALTER TABLE %s ADD %s", QuoteMySQL(c.Table), c.After)
		}
	}
	if l := changesOf(changes, KindTable, Removed); len(l) > 0 {
		w.section("Tables only in the target")
		for _, c := range l {
			w.destructive("drops the table and its rows", "DROP TABLE %s", QuoteMySQL(c.Table))
		}
	}
	if l := changesOf(changes, KindView, Added, Changed); len(l) > 0 {
		w.section("Views")
		for _, c := range l {
			w.stmt("%s", c.After)
		}
	}
	if l := changesOf(changes, KindRoutine, Added, Changed); len(l) > 0 {
		w.section("Routines")
		for _, c := range l {
			mysqlCompound(w, c.After)
		}
	}
	if l := changesOf(changes, KindTrigger, Added, Changed); len(l) > 0 {
		w.section("Triggers")
		for _, c := range l {
			mysqlCompound(w, c.After)
		}
	}
	if tableWork {
		w.raw("\n")
		w.stmt("SET FOREIGN_KEY_CHECKS = 1")
	}
}

// findTableObjects returns the indexes (or constraints) of the table a
// change belongs to.
func findTableObjects(s *Schema, c Change, constraints bool) []Object {
	t := findTable(s, c.Schema, c.Table)
	if t == nil {
		return nil
	}
	if constraints {
		return t.Constraints
	}
	return t.Indexes
}

func findChange(changes []Change, kind, table, name string) *Change {
	for i := range changes {
		c := &changes[i]
		if c.Kind == kind && c.Table == table && c.Name == name {
			return c
		}
	}
	return nil
}

// changedTables lists, in order, the existing tables with column, index or
// option changes.
func changedTables(changes []Change) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range changes {
		switch c.Kind {
		case KindColumn, KindIndex, KindOptions:
			if !seen[c.Schema+"."+c.Table] {
				seen[c.Schema+"."+c.Table] = true
				out = append(out, c.Table)
			}
		}
	}
	return out
}

// ---- PostgreSQL ----

// PGColumnDef renders a PostgreSQL column definition from its parts.
func PGColumnDef(c Column) string {
	s := QuotePG(c.Name) + " " + c.Type
	if c.Collation != "" {
		s += " COLLATE " + c.Collation
	}
	if c.Generated != "" {
		s += " GENERATED ALWAYS AS (" + c.Generated + ") STORED"
	}
	if c.Identity != "" {
		s += " GENERATED " + c.Identity + " AS IDENTITY"
	}
	if c.NotNull {
		s += " NOT NULL"
	}
	if c.Default != "" {
		s += " DEFAULT " + c.Default
	}
	return s
}

func pgCreateTable(t *Table) string {
	var lines []string
	for _, c := range t.Columns {
		lines = append(lines, c.Def)
	}
	s := "CREATE TABLE " + pgTable(t.Schema, t.Name) + " (\n  " + strings.Join(lines, ",\n  ") + "\n)"
	if t.Options != "" {
		s += " " + t.Options
	}
	return s
}

var nextvalRe = regexp.MustCompile(`^nextval\('((?:[^']|'')+)'::regclass\)$`)

// pgSequence returns the sequence a column default draws from, if any.
func pgSequence(c Column) string {
	m := nextvalRe.FindStringSubmatch(c.Default)
	if m == nil {
		return ""
	}
	return strings.ReplaceAll(m[1], "''", "'")
}

// pgOrder ranks constraint kinds: keys before checks before foreign keys.
func pgConstraintRank(kind string) int {
	switch kind {
	case "p":
		return 0
	case "u", "x":
		return 1
	case "c":
		return 2
	}
	return 3
}

func pgScript(w *writer, from, to *Schema, changes []Change) {
	// A new table's constraints and indexes are added like any other.
	for _, c := range changesOf(changes, KindTable, Added) {
		t := findTable(to, c.Schema, c.Table)
		for _, o := range t.Constraints {
			changes = append(changes, Change{Kind: KindConstraint, Op: Added, Schema: t.Schema, Table: t.Name, Name: o.Name, After: o.Def})
		}
		for _, o := range t.Indexes {
			changes = append(changes, Change{Kind: KindIndex, Op: Added, Schema: t.Schema, Table: t.Name, Name: o.Name, After: o.Def})
		}
	}
	if l := changesOf(changes, KindExtension); len(l) > 0 {
		w.section("Extensions")
		for _, c := range l {
			switch c.Op {
			case Added:
				w.stmt("%s", c.After)
			case Removed:
				w.destructive("other objects may depend on it", "DROP EXTENSION %s", QuotePG(c.Name))
			default:
				w.manual("extension %s differs: %s", c.Name, c.After)
			}
		}
	}
	if l := changesOf(changes, KindType); len(l) > 0 {
		w.section("Types")
		for _, c := range l {
			switch c.Op {
			case Added:
				w.stmt("%s", c.After)
			case Removed:
				w.destructive("columns may use it", "DROP TYPE %s", pgTable(c.Schema, c.Name))
			default:
				pgEnumChange(w, findObject(from.Types, c.Schema, "", c.Name), findObject(to.Types, c.Schema, "", c.Name))
			}
		}
	}
	if l := changesOf(changes, KindTrigger, Removed, Changed); len(l) > 0 {
		w.section("Triggers to drop or replace")
		for _, c := range l {
			w.stmt("DROP TRIGGER IF EXISTS %s ON %s", QuotePG(c.Name), pgTable(c.Schema, c.Table))
		}
	}
	if l := changesOf(changes, KindView, Removed); len(l) > 0 {
		w.section("Views to drop")
		for _, c := range l {
			o := findObject(from.Views, c.Schema, "", c.Name)
			kw := "VIEW"
			if o != nil && o.Kind == "MATERIALIZED" {
				kw = "MATERIALIZED VIEW"
			}
			w.stmt("DROP %s IF EXISTS %s", kw, pgTable(c.Schema, c.Name))
		}
	}
	for _, c := range changesOf(changes, KindView, Changed) {
		if o := findObject(from.Views, c.Schema, "", c.Name); o != nil && o.Kind == "MATERIALIZED" {
			w.stmt("DROP MATERIALIZED VIEW IF EXISTS %s", pgTable(c.Schema, c.Name))
		}
	}
	if l := changesOf(changes, KindRoutine, Removed); len(l) > 0 {
		w.section("Routines to drop")
		for _, c := range l {
			if o := findObject(from.Routines, c.Schema, "", c.Name); o != nil && o.Drop != "" {
				w.stmt("%s", o.Drop)
			}
		}
	}
	// Constraints and indexes to drop: foreign keys first.
	drops := changesOf(changes, KindConstraint, Removed, Changed)
	sort.SliceStable(drops, func(i, j int) bool {
		return pgConstraintRank(constraintKind(from, drops[i])) > pgConstraintRank(constraintKind(from, drops[j]))
	})
	if len(drops) > 0 {
		w.section("Constraints to drop or replace")
		for _, c := range drops {
			w.stmt("ALTER TABLE %s DROP CONSTRAINT %s", pgTable(c.Schema, c.Table), QuotePG(c.Name))
		}
	}
	if l := changesOf(changes, KindIndex, Removed, Changed); len(l) > 0 {
		w.section("Indexes to drop or replace")
		for _, c := range l {
			w.stmt("DROP INDEX %s", pgTable(c.Schema, c.Name))
		}
	}
	var owned []string
	ownSeq := func(col Column, schema, table string) {
		if seq := pgSequence(col); seq != "" {
			w.stmt("CREATE SEQUENCE IF NOT EXISTS %s", seq)
			owned = append(owned, fmt.Sprintf("ALTER SEQUENCE %s OWNED BY %s.%s", seq, pgTable(schema, table), QuotePG(col.Name)))
		}
	}
	if l := changesOf(changes, KindTable, Added); len(l) > 0 {
		w.section("New tables")
		for _, c := range l {
			t := findTable(to, c.Schema, c.Table)
			for _, col := range t.Columns {
				ownSeq(col, t.Schema, t.Name)
			}
			w.stmt("%s", pgCreateTable(t))
		}
	}
	for _, c := range changesOf(changes, KindOptions) {
		w.manual("table options of %s differ: %q in the source, %q here", pgTable(c.Schema, c.Table), c.After, c.Before)
	}
	if l := changesOf(changes, KindColumn); len(l) > 0 {
		w.section("Columns")
		for _, c := range l {
			tq := pgTable(c.Schema, c.Table)
			switch c.Op {
			case Added:
				col := findColumn(findTable(to, c.Schema, c.Table), c.Name)
				ownSeq(*col, c.Schema, c.Table)
				w.stmt("ALTER TABLE %s ADD COLUMN %s", tq, col.Def)
			case Removed:
				w.destructive("drops the column's data", "ALTER TABLE %s DROP COLUMN %s", tq, QuotePG(c.Name))
			default:
				pgAlterColumn(w, tq, findColumn(findTable(from, c.Schema, c.Table), c.Name), findColumn(findTable(to, c.Schema, c.Table), c.Name), ownSeq, c)
			}
		}
	}
	adds := changesOf(changes, KindConstraint, Added, Changed)
	sort.SliceStable(adds, func(i, j int) bool {
		return pgConstraintRank(constraintKind(to, adds[i])) < pgConstraintRank(constraintKind(to, adds[j]))
	})
	if len(adds) > 0 {
		w.section("Constraints to add")
		for _, c := range adds {
			w.stmt("ALTER TABLE %s ADD CONSTRAINT %s %s", pgTable(c.Schema, c.Table), QuotePG(c.Name), c.After)
		}
	}
	if l := changesOf(changes, KindIndex, Added, Changed); len(l) > 0 {
		w.section("Indexes")
		for _, c := range l {
			w.stmt("%s", c.After)
		}
	}
	if l := changesOf(changes, KindTable, Removed); len(l) > 0 {
		w.section("Tables only in the target")
		for _, c := range l {
			w.destructive("drops the table and its rows", "DROP TABLE %s", pgTable(c.Schema, c.Table))
		}
	}
	if l := changesOf(changes, KindRoutine, Added, Changed); len(l) > 0 {
		w.section("Routines")
		for _, c := range l {
			if c.Op == Changed {
				w.raw("-- If the return type changed, DROP the routine first.\n")
			}
			w.stmt("%s", c.After)
		}
	}
	if l := changesOf(changes, KindView, Added, Changed); len(l) > 0 {
		w.section("Views")
		for _, c := range l {
			w.stmt("%s", c.After)
		}
	}
	if l := changesOf(changes, KindTrigger, Added, Changed); len(l) > 0 {
		w.section("Triggers")
		for _, c := range l {
			w.stmt("%s", c.After)
		}
	}
	if len(owned) > 0 {
		w.section("Sequence ownership")
		for _, s := range owned {
			w.stmt("%s", s)
		}
	}
}

func constraintKind(s *Schema, c Change) string {
	if o := findObject(findTableObjects(s, c, true), "", "", c.Name); o != nil {
		return o.Kind
	}
	return ""
}

func pgAlterColumn(w *writer, tq string, old, cur *Column, ownSeq func(col Column, schema, table string), c Change) {
	if old == nil || cur == nil {
		return
	}
	col := QuotePG(cur.Name)
	if old.Generated != cur.Generated {
		w.manual("column %s.%s: generation expression changed to %q; drop and re-add the column", tq, col, cur.Generated)
		return
	}
	if old.Type != cur.Type || old.Collation != cur.Collation {
		s := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s", tq, col, cur.Type)
		if cur.Collation != "" {
			s += " COLLATE " + cur.Collation
		}
		w.stmt("%s USING %s::%s", s, col, cur.Type)
	}
	if old.Identity != cur.Identity {
		switch {
		case cur.Identity == "":
			w.stmt("ALTER TABLE %s ALTER COLUMN %s DROP IDENTITY", tq, col)
		case old.Identity == "":
			if old.Default != "" {
				w.stmt("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT", tq, col)
			}
			if !old.NotNull {
				w.stmt("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", tq, col)
			}
			w.stmt("ALTER TABLE %s ALTER COLUMN %s ADD GENERATED %s AS IDENTITY", tq, col, cur.Identity)
			old = &Column{NotNull: true, Identity: cur.Identity}
		default:
			w.stmt("ALTER TABLE %s ALTER COLUMN %s SET GENERATED %s", tq, col, cur.Identity)
		}
	}
	if old.NotNull != cur.NotNull {
		if cur.NotNull {
			w.stmt("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", tq, col)
		} else {
			w.stmt("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL", tq, col)
		}
	}
	if old.Default != cur.Default {
		if cur.Default == "" {
			w.stmt("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT", tq, col)
		} else {
			ownSeq(*cur, c.Schema, c.Table)
			w.stmt("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s", tq, col, cur.Default)
		}
	}
}

// pgEnumChange adds the labels an enum gained, when the old labels are an
// ordered subset of the new ones (PostgreSQL cannot remove or reorder labels).
func pgEnumChange(w *writer, old, cur *Object) {
	if old == nil || cur == nil {
		return
	}
	name := pgTable(cur.Schema, cur.Name)
	j := 0
	for _, v := range cur.Values {
		if j < len(old.Values) && old.Values[j] == v {
			j++
		}
	}
	if j != len(old.Values) || old.Kind != "ENUM" || cur.Kind != "ENUM" {
		w.manual("type %s differs and cannot be altered in place: %s", name, cur.Def)
		return
	}
	have := map[string]bool{}
	for _, v := range old.Values {
		have[v] = true
	}
	for i, v := range cur.Values {
		if have[v] {
			continue
		}
		pos := ""
		if i > 0 {
			pos = " AFTER " + pgLiteral(cur.Values[i-1])
		} else if len(old.Values) > 0 {
			pos = " BEFORE " + pgLiteral(old.Values[0])
		}
		w.stmt("ALTER TYPE %s ADD VALUE %s%s", name, pgLiteral(v), pos)
		have[v] = true
	}
}

func pgLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// PGLiteral quotes a PostgreSQL string literal.
func PGLiteral(s string) string { return pgLiteral(s) }
