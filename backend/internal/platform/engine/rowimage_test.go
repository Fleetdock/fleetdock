package engine

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

// memTable is an in-memory table behind the rowTx interface, so the
// row-image logic can be tested without a database. Statements are not
// parsed: the fake records the last plan and applies it from the request.
type memTable struct {
	meta    *tableMeta
	rows    map[string]RowValues
	nextID  int
	pending func() (int64, RowValues)
}

func sp(s string) *string { return &s }

func keyString(m *tableMeta, k RowValues) string {
	s := ""
	for _, c := range m.key {
		if v := k[c]; v != nil {
			s += *v + "|"
		}
	}
	return s
}

func (t *memTable) read(_ context.Context, m *tableMeta, key RowValues, _ bool) (RowValues, error) {
	r, ok := t.rows[keyString(m, key)]
	if !ok {
		return nil, nil
	}
	cp := RowValues{}
	for c, v := range r {
		cp[c] = v
	}
	return cp, nil
}

func (t *memTable) exec(context.Context, string, []any) (int64, error) {
	n, _ := t.pending()
	return n, nil
}

func (t *memTable) insert(context.Context, *tableMeta, string, []any) (int64, RowValues, error) {
	n, assigned := t.pending()
	return n, assigned, nil
}

func newMem() *memTable {
	cols := []column{
		{Name: "id", Type: "int", AutoIncrement: true, HasDefault: true},
		{Name: "name", Type: "text"},
		{Name: "note", Type: "text", Nullable: true, HasDefault: true},
		{Name: "upper_name", Type: "text", Generated: true, HasDefault: true, Nullable: true},
	}
	m := newTableMeta("`t`", cols, []string{"id"})
	return &memTable{meta: m, rows: map[string]RowValues{}, nextID: 1}
}

// do wires the fake so the next statement applies fn.
func (t *memTable) do(fn func() (int64, RowValues)) { t.pending = fn }

func (t *memTable) put(r RowValues) { t.rows[keyString(t.meta, r)] = r }

func TestChangeRowInsertUpdateDelete(t *testing.T) {
	ctx := context.Background()
	mt := newMem()
	d := mysqlDialect

	// Insert without the id: the database assigns it.
	mt.do(func() (int64, RowValues) {
		id := strconv.Itoa(mt.nextID)
		mt.put(RowValues{"id": sp(id), "name": sp("ann"), "note": nil, "upper_name": sp("ANN")})
		return 1, RowValues{"id": sp(id)}
	})
	ch, err := changeRow(ctx, d, mt, mt.meta, "insert", nil, RowValues{"name": sp("ann")})
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Undoable || *ch.Key["id"] != "1" || ch.Before != nil || *ch.After["upper_name"] != "ANN" {
		t.Fatalf("insert change = %+v", ch)
	}

	// Update keeps only the touched columns plus the key.
	mt.do(func() (int64, RowValues) {
		mt.rows["1|"]["note"] = sp("hi")
		return 1, nil
	})
	ch, err = changeRow(ctx, d, mt, mt.meta, "update", RowValues{"id": sp("1")}, RowValues{"note": sp("hi")})
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Before) != 2 || ch.Before["note"] != nil || *ch.After["note"] != "hi" || ch.Columns[0] != "id" || ch.Columns[1] != "note" {
		t.Fatalf("update change = %+v", ch)
	}

	// Updating a missing row changes nothing.
	if _, err := changeRow(ctx, d, mt, mt.meta, "update", RowValues{"id": sp("9")}, RowValues{"note": sp("x")}); err == nil {
		t.Fatal("updating a missing row must fail")
	}

	// Delete keeps the full row.
	mt.do(func() (int64, RowValues) {
		delete(mt.rows, "1|")
		return 1, nil
	})
	ch, err = changeRow(ctx, d, mt, mt.meta, "delete", RowValues{"id": sp("1")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ch.After != nil || *ch.Before["name"] != "ann" || *ch.Before["note"] != "hi" {
		t.Fatalf("delete change = %+v", ch)
	}
}

func TestChangeRowInsertKeyUnknown(t *testing.T) {
	mt := newMem()
	mt.do(func() (int64, RowValues) { return 1, nil }) // no assigned key reported
	ch, err := changeRow(context.Background(), mysqlDialect, mt, mt.meta, "insert", nil, RowValues{"name": sp("x")})
	if err != nil || ch.Undoable || ch.Note == "" {
		t.Fatalf("change = %+v, %v", ch, err)
	}
}

func TestPlanApply(t *testing.T) {
	m := newMem().meta
	key := RowValues{"id": sp("1")}
	cur := RowValues{"id": sp("1"), "name": sp("ann"), "note": nil, "upper_name": sp("ANN")}

	p, err := mysqlDialect.planApply(m, key, cur, RowValues{"id": sp("1"), "name": sp("ann"), "upper_name": sp("X")})
	if err != nil || p.op != "" {
		t.Fatalf("no-op plan = %+v, %v (generated columns are never written)", p, err)
	}
	p, _ = mysqlDialect.planApply(m, key, cur, RowValues{"note": sp("n")})
	if p.op != "update" || len(p.written) != 1 || p.sql != "UPDATE `t` SET `note` = ? WHERE `id` = ?" {
		t.Fatalf("update plan = %+v", p)
	}
	p, _ = mysqlDialect.planApply(m, key, nil, cur)
	if p.op != "insert" || p.written["upper_name"] != nil || len(p.written) != 3 {
		t.Fatalf("insert plan = %+v", p)
	}
	p, _ = mysqlDialect.planApply(m, key, cur, nil)
	if p.op != "delete" {
		t.Fatalf("delete plan = %+v", p)
	}
	if _, err := mysqlDialect.planApply(m, key, cur, RowValues{"gone": sp("x")}); !errors.Is(err, ErrStructureChanged) {
		t.Fatalf("unknown column err = %v", err)
	}

	pgm := newTableMeta(`"t"`, []column{{Name: "id", Type: "integer", IdentityAlways: true}, {Name: "v", Type: "text"}}, []string{"id"})
	p, _ = pgDialect.planApply(pgm, RowValues{"id": sp("5")}, nil, RowValues{"id": sp("5"), "v": sp("x")})
	if p.sql != `INSERT INTO "t" ("id", "v") OVERRIDING SYSTEM VALUE VALUES (($1::text)::integer, ($2::text)::text)` {
		t.Fatalf("identity insert = %q", p.sql)
	}
}

func TestApplyRowCheck(t *testing.T) {
	ctx := context.Background()
	mt := newMem()
	mt.put(RowValues{"id": sp("1"), "name": sp("bob"), "note": nil, "upper_name": sp("BOB")})
	req := ApplyRequest{Key: RowValues{"id": sp("1")}, Check: ExpectImage(RowValues{"id": sp("1"), "name": sp("ann")}), Want: RowValues{"name": sp("x")}}
	if _, err := applyRow(ctx, mysqlDialect, mt, mt.meta, req); !errors.Is(err, ErrRowChanged) {
		t.Fatalf("err = %v", err)
	}
	// Re-insert a deleted row: expected absent.
	mt.do(func() (int64, RowValues) {
		mt.put(RowValues{"id": sp("7"), "name": sp("zed"), "note": nil, "upper_name": sp("ZED")})
		return 1, nil
	})
	ch, err := applyRow(ctx, mysqlDialect, mt, mt.meta, ApplyRequest{
		Key: RowValues{"id": sp("7")}, Check: ExpectImage(nil), Want: RowValues{"id": sp("7"), "name": sp("zed")},
	})
	if err != nil || ch.Op != "insert" || *ch.After["upper_name"] != "ZED" {
		t.Fatalf("reinsert = %+v, %v", ch, err)
	}
	if _, err := applyRow(ctx, mysqlDialect, mt, mt.meta, ApplyRequest{
		Key: RowValues{"id": sp("7")}, Check: ExpectImage(nil), Want: RowValues{"id": sp("7")},
	}); !errors.Is(err, ErrRowChanged) {
		t.Fatalf("re-inserting over an existing row must fail, got %v", err)
	}
}
