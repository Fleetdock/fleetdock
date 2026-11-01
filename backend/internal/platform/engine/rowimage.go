package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Row images: what a row looked like before and after a change made through
// the data browser, captured in the same transaction as the change, so the
// change can be listed and undone later.

// RowChange describes one row change.
type RowChange struct {
	// Op is "insert", "update" or "delete".
	Op string
	// Key identifies the row after the change (before it, for a delete).
	Key RowValues
	// Before and After are the row's values, full and untruncated. An
	// update keeps only the columns it set (plus the key); a delete has no
	// After, an insert no Before.
	Before RowValues
	After  RowValues
	// Columns lists the image columns in table order.
	Columns []string
	// Session is the session settings the images were read under (time
	// zone, date style), re-applied when an image is written back so
	// timestamps mean the same instant.
	Session map[string]string
	// Undoable is false when the row could not be identified after the
	// change (Note says why); the change itself succeeded.
	Undoable bool
	Note     string
}

// ErrRowChanged is returned when a row no longer looks as expected: someone
// changed it after the change being undone.
var ErrRowChanged = errors.New("the row was changed since; undoing would overwrite that change")

// ErrStructureChanged is returned when an image names a column the table no
// longer has.
var ErrStructureChanged = errors.New("the table's columns changed since; this change cannot be applied automatically")

// ApplyRequest makes the row at Key look like Want.
type ApplyRequest struct {
	// Key identifies the row as it is now. For a row that should not exist
	// yet (re-inserting a deleted row) it is the key it will have.
	Key RowValues
	// Check is called with the row as it is now (nil = absent) under a row
	// lock; an error aborts. Nil skips the check (force).
	Check func(current RowValues) error
	// Want is the values to write; nil deletes the row. Columns not in
	// Want are left as they are.
	Want RowValues
	// Session settings to apply first (from RowChange.Session).
	Session map[string]string
}

// ExpectImage returns a Check that requires every column of want to hold the
// same value now (absent want = row must be absent).
func ExpectImage(want RowValues) func(RowValues) error {
	return func(current RowValues) error {
		if want == nil {
			if current != nil {
				return ErrRowChanged
			}
			return nil
		}
		if current == nil {
			return ErrRowChanged
		}
		if !SameValues(want, current) {
			return ErrRowChanged
		}
		return nil
	}
}

// SameValues reports whether every column of want has the same value in got.
func SameValues(want, got RowValues) bool {
	for c, v := range want {
		g, ok := got[c]
		if !ok || (v == nil) != (g == nil) || (v != nil && *v != *g) {
			return false
		}
	}
	return true
}

// keyOf extracts the row key from values, or nil if a key column is missing
// or NULL.
func (m *tableMeta) keyOf(values RowValues) RowValues {
	if len(m.key) == 0 {
		return nil
	}
	k := make(RowValues, len(m.key))
	for _, c := range m.key {
		v, ok := values[c]
		if !ok || v == nil {
			return nil
		}
		k[c] = v
	}
	return k
}

// overlay returns base with the columns of top replacing its own.
func overlay(base, top RowValues) RowValues {
	out := make(RowValues, len(base)+len(top))
	for c, v := range base {
		out[c] = v
	}
	for c, v := range top {
		out[c] = v
	}
	return out
}

// pick returns the columns of img named in cols plus the key columns.
func (m *tableMeta) pick(img RowValues, cols RowValues) RowValues {
	out := RowValues{}
	for c := range cols {
		if v, ok := img[c]; ok {
			out[c] = v
		}
	}
	for _, c := range m.key {
		if v, ok := img[c]; ok {
			out[c] = v
		}
	}
	return out
}

// imageColumns lists the columns of the images in table order.
func (m *tableMeta) imageColumns(imgs ...RowValues) []string {
	var out []string
	for _, c := range m.columns {
		for _, img := range imgs {
			if _, ok := img[c.Name]; ok {
				out = append(out, c.Name)
				break
			}
		}
	}
	return out
}

// selectByKeySQL reads one row's full values by key; forUpdate locks it.
func (d dialect) selectByKeySQL(m *tableMeta, key RowValues, forUpdate bool) (string, []any, error) {
	where, args, err := d.keyWhere(m, key, nil)
	if err != nil {
		return "", nil, err
	}
	names := make([]string, len(m.columns))
	for i, c := range m.columns {
		names[i] = d.quote(c.Name)
	}
	q := fmt.Sprintf("SELECT %s FROM %s WHERE %s", strings.Join(names, ", "), m.qualified, where)
	if forUpdate {
		q += " FOR UPDATE"
	}
	return q, args, nil
}

// rowToValues maps a selectByKeySQL row to column names.
func (m *tableMeta) rowToValues(row []*string) RowValues {
	out := make(RowValues, len(m.columns))
	for i, c := range m.columns {
		if i < len(row) {
			out[c.Name] = row[i]
		}
	}
	return out
}

// applyPlan is the statement that makes a row match an ApplyRequest.
type applyPlan struct {
	op   string // insert, update, delete, or "" (already as wanted)
	sql  string
	args []any
	// written is the subset of Want actually written.
	written RowValues
}

// planApply decides how to turn current into want. Generated columns are
// never written; an identity column that only accepts generated values is
// written with OVERRIDING SYSTEM VALUE (PostgreSQL).
func (d dialect) planApply(m *tableMeta, key, current, want RowValues) (*applyPlan, error) {
	for c := range want {
		if _, ok := m.byName[c]; !ok {
			return nil, ErrStructureChanged
		}
	}
	writable := RowValues{}
	for c, v := range want {
		if !m.byName[c].Generated {
			writable[c] = v
		}
	}
	switch {
	case want == nil && current == nil:
		return &applyPlan{}, nil
	case want == nil:
		q, args, err := d.deleteSQL(m, key)
		return &applyPlan{op: "delete", sql: q, args: args}, err
	case current == nil:
		q, args, err := d.insertSQL(m, writable)
		if err != nil {
			return nil, err
		}
		for c := range writable {
			if m.byName[c].IdentityAlways {
				q = strings.Replace(q, ") VALUES (", ") OVERRIDING SYSTEM VALUE VALUES (", 1)
				break
			}
		}
		return &applyPlan{op: "insert", sql: q, args: args, written: writable}, nil
	}
	changed := RowValues{}
	for c, v := range writable {
		if cur, ok := current[c]; !ok || (v == nil) != (cur == nil) || (v != nil && *v != *cur) {
			changed[c] = v
		}
	}
	if len(changed) == 0 {
		return &applyPlan{}, nil
	}
	q, args, err := d.updateSQL(m, key, changed)
	return &applyPlan{op: "update", sql: q, args: args, written: changed}, err
}

// rowTx is one engine's transaction as the row-image logic needs it.
type rowTx interface {
	// read returns the row with key (full values) or nil; forUpdate locks it.
	read(ctx context.Context, m *tableMeta, key RowValues, forUpdate bool) (RowValues, error)
	// exec runs a single-row UPDATE or DELETE and returns the rows affected.
	exec(ctx context.Context, q string, args []any) (int64, error)
	// insert runs an INSERT and returns the rows affected plus the values of
	// key columns the database assigned (AUTO_INCREMENT, serial, identity).
	insert(ctx context.Context, m *tableMeta, q string, args []any) (int64, RowValues, error)
}

// changeRow performs a data-browser edit (op insert, update or delete) of
// exactly one row inside tx and captures its images.
func changeRow(ctx context.Context, d dialect, tx rowTx, m *tableMeta, op string, key, values RowValues) (*RowChange, error) {
	ch := &RowChange{Op: op, Undoable: true}
	var before RowValues
	if op != "insert" {
		k := key
		if _, _, err := d.keyWhere(m, k, nil); err != nil {
			return nil, err
		}
		var err error
		if before, err = tx.read(ctx, m, k, true); err != nil {
			return nil, err
		}
		if before == nil {
			return nil, exactlyOne(0)
		}
	}
	switch op {
	case "insert":
		q, args, err := d.insertSQL(m, values)
		if err != nil {
			return nil, err
		}
		n, assigned, err := tx.insert(ctx, m, q, args)
		if err != nil {
			return nil, err
		}
		if err := exactlyOne(n); err != nil {
			return nil, err
		}
		ch.Key = m.keyOf(overlay(values, assigned))
	case "update":
		q, args, err := d.updateSQL(m, key, values)
		if err != nil {
			return nil, err
		}
		n, err := tx.exec(ctx, q, args)
		if err != nil {
			return nil, err
		}
		if err := exactlyOne(n); err != nil {
			return nil, err
		}
		ch.Key = m.keyOf(overlay(before, values))
		ch.Before = m.pick(before, values)
	case "delete":
		q, args, err := d.deleteSQL(m, key)
		if err != nil {
			return nil, err
		}
		n, err := tx.exec(ctx, q, args)
		if err != nil {
			return nil, err
		}
		if err := exactlyOne(n); err != nil {
			return nil, err
		}
		ch.Key, ch.Before = m.keyOf(before), before
	default:
		return nil, fmt.Errorf("unknown change %q", op)
	}
	if op != "delete" {
		if ch.Key == nil {
			ch.Undoable, ch.Note = false, "the new row's key is assigned by the database in a way Fleetdock cannot read back"
			return ch, nil
		}
		after, err := tx.read(ctx, m, ch.Key, false)
		if err != nil {
			return nil, err
		}
		if after == nil {
			// A trigger moved or removed the row.
			ch.Undoable, ch.Note = false, "the row is no longer at its key after the change (a trigger may have changed it)"
			return ch, nil
		}
		if op == "update" {
			ch.After = m.pick(after, values)
		} else {
			ch.After = after
		}
	}
	ch.Columns = m.imageColumns(ch.Before, ch.After)
	return ch, nil
}

// applyRow makes the row at req.Key match req.Want inside tx (see
// ApplyRequest) and describes what it changed. An Op of "" means the row
// already matched.
func applyRow(ctx context.Context, d dialect, tx rowTx, m *tableMeta, req ApplyRequest) (*RowChange, error) {
	if _, _, err := d.keyWhere(m, req.Key, nil); err != nil {
		return nil, err
	}
	current, err := tx.read(ctx, m, req.Key, true)
	if err != nil {
		return nil, err
	}
	if req.Check != nil {
		if err := req.Check(current); err != nil {
			return nil, err
		}
	}
	plan, err := d.planApply(m, req.Key, current, req.Want)
	if err != nil {
		return nil, err
	}
	ch := &RowChange{Op: plan.op, Key: req.Key, Undoable: true}
	switch plan.op {
	case "":
		return ch, nil
	case "insert":
		n, assigned, err := tx.insert(ctx, m, plan.sql, plan.args)
		if err != nil {
			return nil, err
		}
		if err := exactlyOne(n); err != nil {
			return nil, err
		}
		if k := m.keyOf(overlay(plan.written, assigned)); k != nil {
			ch.Key = k
		}
	default:
		n, err := tx.exec(ctx, plan.sql, plan.args)
		if err != nil {
			return nil, err
		}
		if err := exactlyOne(n); err != nil {
			return nil, err
		}
	}
	switch plan.op {
	case "delete":
		ch.Before = current
	case "update":
		ch.Key = m.keyOf(overlay(current, plan.written))
		ch.Before = m.pick(current, plan.written)
		after, err := tx.read(ctx, m, ch.Key, false)
		if err != nil {
			return nil, err
		}
		ch.After = m.pick(after, plan.written)
	case "insert":
		after, err := tx.read(ctx, m, ch.Key, false)
		if err != nil {
			return nil, err
		}
		ch.After = after
	}
	ch.Columns = m.imageColumns(ch.Before, ch.After)
	return ch, nil
}
