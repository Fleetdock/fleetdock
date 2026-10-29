package engine

import (
	"reflect"
	"testing"
)

func TestPreviewBlocker(t *testing.T) {
	for _, ok := range []string{
		"SELECT 1", "UPDATE t SET a = 1", "delete from t", "INSERT INTO t VALUES (1)", "REPLACE INTO t VALUES (1)",
		"WITH x AS (SELECT 1) SELECT * FROM x", "MERGE INTO t USING s ON true WHEN MATCHED THEN DELETE",
		"SELECT 'into outfile' AS s",
	} {
		if err := PreviewBlocker(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"CREATE TABLE t (a int)", "DROP TABLE t", "ALTER TABLE t ADD c int", "TRUNCATE t", "COMMIT", "BEGIN",
		"ROLLBACK", "SAVEPOINT a", "SET autocommit = 1", "CALL p()", "LOCK TABLES t WRITE", "GRANT ALL ON t TO u",
		"LOAD DATA INFILE 'x' INTO TABLE t", "SELECT * FROM t INTO OUTFILE '/tmp/x'", "DO 1", "VACUUM t",
	} {
		if err := PreviewBlocker(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}

func TestParseSingleTableDML(t *testing.T) {
	cases := []struct {
		stmt  string
		ok    bool
		parts []string
		alias string
		cond  string
	}{
		{"UPDATE items SET qty = 2 WHERE id = 1", true, []string{"items"}, "", "id = 1"},
		{"update `shop`.`items` i set i.qty = 2 where i.sku like 'A%'", true, []string{"shop", "items"}, "i", "i.sku like 'A%'"},
		{`UPDATE ONLY "public"."My ""T""" AS t SET a = 1 WHERE t.a > 0 RETURNING *`, true, []string{"public", `My "T"`}, "t", "t.a > 0"},
		{"UPDATE t SET note = 'where id = 1, or not' WHERE (id IN (SELECT id FROM o WHERE x = 1))", true, []string{"t"}, "", "(id IN (SELECT id FROM o WHERE x = 1))"},
		{"UPDATE t SET a = 1", true, []string{"t"}, "", ""},
		{"DELETE FROM t WHERE a = 1 -- trailing comment", true, []string{"t"}, "", "a = 1 -- trailing comment"},
		{"DELETE LOW_PRIORITY FROM t AS x WHERE x.a = $1$y$1$", true, []string{"t"}, "x", "x.a = $1$y$1$"},
		{"DELETE FROM t WHERE a = 1 RETURNING id", true, []string{"t"}, "", "a = 1"},
		{"DELETE FROM t", true, []string{"t"}, "", ""},
		// Not recognised: counts only.
		{"UPDATE t1, t2 SET t1.a = t2.a WHERE t1.id = t2.id", false, nil, "", ""},
		{"UPDATE t JOIN u ON u.id = t.u SET t.a = 1", false, nil, "", ""},
		{"UPDATE t SET a = 1 FROM u WHERE u.id = t.id", false, nil, "", ""},
		{"UPDATE t SET a = 1 WHERE b = 2 ORDER BY id LIMIT 10", false, nil, "", ""},
		{"DELETE t1 FROM t1 JOIN t2 ON t1.id = t2.id", false, nil, "", ""},
		{"DELETE FROM t USING u WHERE t.id = u.id", false, nil, "", ""},
		{"DELETE FROM t WHERE a = 1 LIMIT 5", false, nil, "", ""},
		{"INSERT INTO t VALUES (1)", false, nil, "", ""},
	}
	for _, c := range cases {
		got, ok := parseSingleTableDML(c.stmt)
		if ok != c.ok {
			t.Errorf("%q: ok = %v, want %v", c.stmt, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if !reflect.DeepEqual(got.parts, c.parts) || got.alias != c.alias || got.cond != c.cond {
			t.Errorf("%q: got parts %q alias %q cond %q", c.stmt, got.parts, got.alias, got.cond)
		}
	}
}

func TestSampleSQL(t *testing.T) {
	d, _ := parseSingleTableDML("UPDATE `items` i SET i.qty = 2 WHERE i.qty > 1")
	if got := d.sampleSQL(20); got != "SELECT * FROM `items` i WHERE (i.qty > 1\n)\nLIMIT 21" {
		t.Fatalf("sample = %q", got)
	}
	d, _ = parseSingleTableDML("DELETE FROM t WHERE a = 1 -- why")
	if got := d.sampleSQL(5); got != "SELECT * FROM t WHERE (a = 1 -- why\n)\nLIMIT 6" {
		t.Fatalf("sample = %q", got)
	}
}
