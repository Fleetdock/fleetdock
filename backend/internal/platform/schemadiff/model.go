// Package schemadiff models a database's structure (tables, columns,
// indexes, constraints, views, routines, triggers, types), fingerprints it,
// diffs two structures and writes the DDL that turns one into the other.
//
// A Schema is engine-specific text: definitions are kept as the server
// prints them (SHOW CREATE on MySQL/MariaDB, pg_get_*def on PostgreSQL) after
// canonicalisation, so two databases on the same engine compare equal exactly
// when their structure is the same. It never imports the engine package; the
// engines fill it in.
package schemadiff

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// Dialects.
const (
	MySQL    = "mysql" // MySQL and MariaDB
	Postgres = "postgres"
)

// Schema is one database's structure.
type Schema struct {
	// Dialect is MySQL or Postgres; Engine the exact engine (mysql, mariadb,
	// postgres). Schemas only compare across the same engine.
	Dialect    string   `json:"dialect"`
	Engine     string   `json:"engine"`
	Tables     []Table  `json:"tables"`
	Views      []Object `json:"views,omitempty"`
	Routines   []Object `json:"routines,omitempty"`
	Triggers   []Object `json:"triggers,omitempty"`
	Types      []Object `json:"types,omitempty"`
	Extensions []Object `json:"extensions,omitempty"`
	// Truncated: the database has more objects than are read; only the
	// first ones are in the schema.
	Truncated bool `json:"truncated,omitempty"`
}

// Table is a base table.
type Table struct {
	Schema      string   `json:"schema,omitempty"` // PostgreSQL schema
	Name        string   `json:"name"`
	Columns     []Column `json:"columns"` // in ordinal order
	Indexes     []Object `json:"indexes,omitempty"`
	Constraints []Object `json:"constraints,omitempty"`
	// Options are the table options after the column list (MySQL: ENGINE,
	// CHARSET, COLLATE, COMMENT, partitioning; PostgreSQL: PARTITION BY).
	Options string `json:"options,omitempty"`
}

// Column is a table column. Def is its full definition and what is
// compared; the other fields break it down for engines whose ALTER needs the
// parts (PostgreSQL).
type Column struct {
	Name      string `json:"name"`
	Def       string `json:"def"`
	Type      string `json:"type,omitempty"`
	Collation string `json:"collation,omitempty"`
	NotNull   bool   `json:"not_null,omitempty"`
	Default   string `json:"default,omitempty"`
	Identity  string `json:"identity,omitempty"`  // "ALWAYS" or "BY DEFAULT"
	Generated string `json:"generated,omitempty"` // generation expression
}

// Object is any other named definition: an index, a constraint, a view, a
// routine, a trigger, a type or an extension.
type Object struct {
	Schema string `json:"schema,omitempty"`
	Name   string `json:"name"`
	// Kind refines the object: PRIMARY/UNIQUE/INDEX for MySQL indexes,
	// p/u/c/f/x for PostgreSQL constraints, FOREIGN/CHECK for MySQL
	// constraints, PROCEDURE/FUNCTION for routines, VIEW/MATERIALIZED for
	// views, ENUM for types.
	Kind string `json:"kind,omitempty"`
	// Table is the table a trigger belongs to.
	Table string `json:"table,omitempty"`
	Def   string `json:"def"`
	// Values are an enum type's labels in order.
	Values []string `json:"values,omitempty"`
	// Drop is the statement that removes the object, when it is not derived
	// from the names (PostgreSQL functions need their argument types).
	Drop string `json:"drop,omitempty"`
}

// Canonicalize sorts every list by name so equal structures serialise
// equally. Columns keep their ordinal order.
func (s *Schema) Canonicalize() {
	sort.Slice(s.Tables, func(i, j int) bool {
		return less(s.Tables[i].Schema, s.Tables[i].Name, s.Tables[j].Schema, s.Tables[j].Name)
	})
	for i := range s.Tables {
		sortObjects(s.Tables[i].Indexes)
		sortObjects(s.Tables[i].Constraints)
	}
	for _, l := range [][]Object{s.Views, s.Routines, s.Triggers, s.Types, s.Extensions} {
		sortObjects(l)
	}
	if s.Tables == nil {
		s.Tables = []Table{}
	}
}

func less(s1, n1, s2, n2 string) bool {
	if s1 != s2 {
		return s1 < s2
	}
	return n1 < n2
}

func sortObjects(l []Object) {
	sort.Slice(l, func(i, j int) bool {
		if l[i].Schema != l[j].Schema {
			return l[i].Schema < l[j].Schema
		}
		if l[i].Table != l[j].Table {
			return l[i].Table < l[j].Table
		}
		return l[i].Name < l[j].Name
	})
}

// Hash fingerprints the structure: equal for equal structures, whatever the
// data, the AUTO_INCREMENT counters or the column order.
func (s *Schema) Hash() string {
	c := *s
	c.Engine = ""
	c.Truncated = false
	c.Tables = make([]Table, len(s.Tables))
	for i, t := range s.Tables {
		t.Columns = append([]Column(nil), t.Columns...)
		sort.Slice(t.Columns, func(a, b int) bool { return t.Columns[a].Name < t.Columns[b].Name })
		c.Tables[i] = t
	}
	c.Canonicalize()
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Counts summarises a schema.
type Counts struct {
	Tables   int `json:"tables"`
	Views    int `json:"views"`
	Routines int `json:"routines"`
	Triggers int `json:"triggers"`
}

// Counts returns how many objects of each kind the schema has.
func (s *Schema) Counts() Counts {
	return Counts{Tables: len(s.Tables), Views: len(s.Views), Routines: len(s.Routines), Triggers: len(s.Triggers)}
}
