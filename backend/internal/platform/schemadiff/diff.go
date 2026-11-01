package schemadiff

import (
	"fmt"
	"strings"
)

// Change kinds.
const (
	KindTable      = "table"
	KindColumn     = "column"
	KindIndex      = "index"
	KindConstraint = "constraint"
	KindOptions    = "options"
	KindView       = "view"
	KindRoutine    = "routine"
	KindTrigger    = "trigger"
	KindType       = "type"
	KindExtension  = "extension"
)

// Change ops.
const (
	Added   = "added"
	Removed = "removed"
	Changed = "changed"
)

// Change is one structural difference between two schemas.
type Change struct {
	Kind string `json:"kind"`
	Op   string `json:"op"`
	// Schema and Table locate the object (Table is empty for views,
	// routines, types and extensions); Name names it (empty for table-level
	// changes).
	Schema string `json:"schema,omitempty"`
	Table  string `json:"table,omitempty"`
	Name   string `json:"name,omitempty"`
	// Before and After are the definitions on each side.
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// Label is a short human description: "column items.qty".
func (c Change) Label() string {
	q := c.Table
	if c.Schema != "" && c.Schema != "public" && q != "" {
		q = c.Schema + "." + q
	}
	switch c.Kind {
	case KindTable, KindOptions:
		return "table " + q
	case KindColumn, KindIndex, KindConstraint, KindTrigger:
		return fmt.Sprintf("%s %s.%s", c.Kind, q, c.Name)
	}
	n := c.Name
	if c.Schema != "" && c.Schema != "public" {
		n = c.Schema + "." + n
	}
	return c.Kind + " " + n
}

// Diff lists what changed from a to b: Added is in b only, Removed in a only.
func Diff(a, b *Schema) []Change {
	var out []Change
	at := map[string]*Table{}
	for i := range a.Tables {
		at[key(a.Tables[i].Schema, a.Tables[i].Name)] = &a.Tables[i]
	}
	bt := map[string]*Table{}
	for i := range b.Tables {
		bt[key(b.Tables[i].Schema, b.Tables[i].Name)] = &b.Tables[i]
	}
	for i := range b.Tables {
		t := &b.Tables[i]
		old, ok := at[key(t.Schema, t.Name)]
		if !ok {
			out = append(out, Change{Kind: KindTable, Op: Added, Schema: t.Schema, Table: t.Name, After: CreateTable(b.Dialect, t)})
			continue
		}
		out = append(out, diffTable(a.Dialect, old, t)...)
	}
	for i := range a.Tables {
		t := &a.Tables[i]
		if _, ok := bt[key(t.Schema, t.Name)]; !ok {
			out = append(out, Change{Kind: KindTable, Op: Removed, Schema: t.Schema, Table: t.Name, Before: CreateTable(a.Dialect, t)})
		}
	}
	out = append(out, diffObjects(KindView, "", a.Views, b.Views)...)
	out = append(out, diffObjects(KindRoutine, "", a.Routines, b.Routines)...)
	out = append(out, diffObjects(KindTrigger, "", a.Triggers, b.Triggers)...)
	out = append(out, diffObjects(KindType, "", a.Types, b.Types)...)
	out = append(out, diffObjects(KindExtension, "", a.Extensions, b.Extensions)...)
	return out
}

func key(parts ...string) string { return strings.Join(parts, "\x00") }

func diffTable(_ string, a, b *Table) []Change {
	var out []Change
	if a.Options != b.Options {
		out = append(out, Change{Kind: KindOptions, Op: Changed, Schema: b.Schema, Table: b.Name, Before: a.Options, After: b.Options})
	}
	ac := map[string]Column{}
	for _, c := range a.Columns {
		ac[c.Name] = c
	}
	bc := map[string]bool{}
	for _, c := range b.Columns {
		bc[c.Name] = true
		old, ok := ac[c.Name]
		switch {
		case !ok:
			out = append(out, Change{Kind: KindColumn, Op: Added, Schema: b.Schema, Table: b.Name, Name: c.Name, After: c.Def})
		case old.Def != c.Def:
			out = append(out, Change{Kind: KindColumn, Op: Changed, Schema: b.Schema, Table: b.Name, Name: c.Name, Before: old.Def, After: c.Def})
		}
	}
	for _, c := range a.Columns {
		if !bc[c.Name] {
			out = append(out, Change{Kind: KindColumn, Op: Removed, Schema: b.Schema, Table: b.Name, Name: c.Name, Before: c.Def})
		}
	}
	for _, ch := range diffObjects(KindIndex, b.Name, a.Indexes, b.Indexes) {
		ch.Schema = b.Schema
		out = append(out, ch)
	}
	for _, ch := range diffObjects(KindConstraint, b.Name, a.Constraints, b.Constraints) {
		ch.Schema = b.Schema
		out = append(out, ch)
	}
	return out
}

func objectKey(o Object) string { return key(o.Schema, o.Table, o.Name) }

func diffObjects(kind, table string, a, b []Object) []Change {
	var out []Change
	am := map[string]Object{}
	for _, o := range a {
		am[objectKey(o)] = o
	}
	bm := map[string]bool{}
	for _, o := range b {
		bm[objectKey(o)] = true
		t := table
		if t == "" {
			t = o.Table
		}
		old, ok := am[objectKey(o)]
		switch {
		case !ok:
			out = append(out, Change{Kind: kind, Op: Added, Schema: o.Schema, Table: t, Name: o.Name, After: o.Def})
		case old.Def != o.Def:
			out = append(out, Change{Kind: kind, Op: Changed, Schema: o.Schema, Table: t, Name: o.Name, Before: old.Def, After: o.Def})
		}
	}
	for _, o := range a {
		if !bm[objectKey(o)] {
			t := table
			if t == "" {
				t = o.Table
			}
			out = append(out, Change{Kind: kind, Op: Removed, Schema: o.Schema, Table: t, Name: o.Name, Before: o.Def})
		}
	}
	return out
}

// Tally counts changes by op.
type Tally struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Changed int `json:"changed"`
}

// Count tallies changes.
func Count(changes []Change) Tally {
	var t Tally
	for _, c := range changes {
		switch c.Op {
		case Added:
			t.Added++
		case Removed:
			t.Removed++
		default:
			t.Changed++
		}
	}
	return t
}
