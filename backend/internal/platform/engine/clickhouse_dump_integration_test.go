//go:build integration

package engine

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// chChecksum is an order-independent fingerprint of a table's rows.
func chChecksum(t *testing.T, c *ClickHouse, p ConnParams, db, table string) string {
	t.Helper()
	res := mustQuery(t, c, p, db, "SELECT count(), sum(cityHash64(*)) FROM "+quoteCHIdent(table), false)
	return *res.Rows[0][0] + "/" + *res.Rows[0][1]
}

func TestIntegrationClickHouseDumpRestore(t *testing.T) {
	c, p := chIT(t)
	ctx := context.Background()
	const src = "fd_it_dump_src"
	_ = c.DropDatabase(ctx, p, src)
	if err := c.CreateDatabase(ctx, p, src, "", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.DropDatabase(context.Background(), p, src) })

	for _, stmt := range []string{
		`CREATE TABLE events (
			id UInt64, kind LowCardinality(String), amount Nullable(Decimal(18, 4)),
			at DateTime64(3, 'UTC'), tags Array(String), attrs Map(String, UInt32),
			doubled UInt64 MATERIALIZED id * 2
		) ENGINE = MergeTree ORDER BY id`,
		`INSERT INTO events (id, kind, amount, at, tags, attrs)
			SELECT number, ['click', 'view', 'buy'][number % 3 + 1], if(number % 7 = 0, NULL, number / 3),
			       toDateTime64('2026-01-01 00:00:00', 3, 'UTC') + number, [toString(number)], map('n', toUInt32(number))
			FROM numbers(25000)`,
		`CREATE TABLE totals (kind String, n UInt64) ENGINE = SummingMergeTree ORDER BY kind`,
		`CREATE MATERIALIZED VIEW totals_mv TO totals AS SELECT kind, count() AS n FROM events GROUP BY kind`,
		`CREATE MATERIALIZED VIEW by_kind ENGINE = SummingMergeTree ORDER BY kind AS SELECT kind, count() AS n FROM events GROUP BY kind`,
		`INSERT INTO events (id, kind, amount, at, tags, attrs) VALUES (99999, 'click', 1, now64(), [], map())`,
		`CREATE VIEW clicks AS SELECT id FROM events WHERE kind = 'click'`,
		`CREATE VIEW clicks_count AS SELECT count() AS n FROM clicks`,
		`CREATE TABLE scratch_mem (x UInt8, s String) ENGINE = Memory`,
		`INSERT INTO scratch_mem VALUES (1, 'it''s'), (2, 'back\\slash')`,
		`CREATE TABLE log_t (x UInt8) ENGINE = TinyLog`,
		`INSERT INTO log_t VALUES (7)`,
		`CREATE TABLE sink (x UInt8) ENGINE = Null`,
		`CREATE DICTIONARY kinds (kind String, n UInt64) PRIMARY KEY kind
			SOURCE(CLICKHOUSE(DB '` + src + `' TABLE 'totals' USER 'default' PASSWORD '` + p.Password + `'))
			LAYOUT(COMPLEX_KEY_HASHED()) LIFETIME(0)`,
	} {
		mustQuery(t, c, p, src, stmt, true)
	}
	want := map[string]string{}
	for _, tbl := range []string{"events", "totals", "by_kind", "scratch_mem", "log_t"} {
		want[tbl] = chChecksum(t, c, p, src, tbl)
	}

	var dump bytes.Buffer
	var logs []string
	if err := c.Dump(ctx, p, src, &dump, func(s string) { logs = append(logs, s) }); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dump.String(), chDumpMagic) {
		t.Fatal("dump lacks the magic header")
	}
	if !strings.Contains(strings.Join(logs, "\n"), "warning: kinds: the server hides the credentials") {
		t.Fatalf("masked credentials should be reported, logs:\n%s", strings.Join(logs, "\n"))
	}

	check := func(t *testing.T, db string) {
		t.Helper()
		for tbl, sum := range want {
			if got := chChecksum(t, c, p, db, tbl); got != sum {
				t.Errorf("%s.%s = %s, want %s", db, tbl, got, sum)
			}
		}
		// Materialized columns are recomputed, views work and point at this
		// database, and the dictionary loads.
		if r := mustQuery(t, c, p, db, "SELECT doubled FROM events WHERE id = 21", false); *r.Rows[0][0] != "42" {
			t.Errorf("materialized column = %s", *r.Rows[0][0])
		}
		if r := mustQuery(t, c, p, db, "SELECT n FROM clicks_count", false); *r.Rows[0][0] != "8335" {
			t.Errorf("view over a view = %s", *r.Rows[0][0])
		}
		ddl := mustQuery(t, c, p, db, "SELECT create_table_query FROM system.tables WHERE database = currentDatabase() AND name = 'clicks'", false)
		if !strings.Contains(*ddl.Rows[0][0], "FROM "+db+".events") {
			t.Errorf("view definition not rewritten: %s", *ddl.Rows[0][0])
		}
		// The server masks the dictionary's password, so it is restored (and
		// reported) without it.
		if r := mustQuery(t, c, p, db, "SELECT count() FROM system.dictionaries WHERE database = currentDatabase() AND name = 'kinds'", false); *r.Rows[0][0] != "1" {
			t.Error("dictionary not restored")
		}
		// The TO view still feeds its table.
		before := chChecksum(t, c, p, db, "totals")
		mustQuery(t, c, p, db, "INSERT INTO events (id, kind, amount, at, tags, attrs) VALUES (123456, 'buy', 1, now64(), [], map())", true)
		if chChecksum(t, c, p, db, "totals") == before {
			t.Error("the materialized view did not fire after restore")
		}
	}

	t.Run("same name, over the original", func(t *testing.T) {
		if err := c.Restore(ctx, p, src, bytes.NewReader(dump.Bytes()), func(string) {}); err != nil {
			t.Fatal(err)
		}
		check(t, src)
	})

	t.Run("new name", func(t *testing.T) {
		const dst = "fd_it_dump_copy"
		_ = c.DropDatabase(ctx, p, dst)
		if err := c.CreateDatabase(ctx, p, dst, "", ""); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.DropDatabase(context.Background(), p, dst) })
		// The restore must not be fed by the source: its rows were already
		// changed above, so compare against the dumped state.
		if err := c.Restore(ctx, p, dst, bytes.NewReader(dump.Bytes()), func(string) {}); err != nil {
			t.Fatal(err)
		}
		check(t, dst)
	})

	t.Run("truncated", func(t *testing.T) {
		const dst = "fd_it_dump_trunc"
		_ = c.CreateDatabase(ctx, p, dst, "", "")
		t.Cleanup(func() { _ = c.DropDatabase(context.Background(), p, dst) })
		cut := dump.Bytes()[:dump.Len()/2]
		if err := c.Restore(ctx, p, dst, bytes.NewReader(cut), func(string) {}); err == nil {
			t.Fatal("a truncated dump must fail")
		}
		if err := c.Restore(ctx, p, dst, strings.NewReader("CREATE TABLE x (a UInt8) ENGINE = Memory;"), func(string) {}); err == nil ||
			!strings.Contains(err.Error(), "not a Fleetdock ClickHouse backup") {
			t.Fatalf("plain SQL must be refused, got %v", err)
		}
	})
}
