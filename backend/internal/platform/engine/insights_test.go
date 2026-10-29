package engine

import "testing"

func ptr64(n int64) *int64 { return &n }

func findings(fs []IndexFinding) map[string]string {
	out := map[string]string{}
	for _, f := range fs {
		out[f.Index.Name] = f.CoveredBy
	}
	return out
}

func TestAnalyzeIndexes(t *testing.T) {
	ix := func(name string, cols ...string) IndexDef {
		return IndexDef{Table: "items", Name: name, Columns: cols, Method: "BTREE", Scans: ptr64(5)}
	}
	pk := ix("PRIMARY", "id")
	pk.Primary, pk.Unique = true, true
	region := ix("ix_region", "region")
	regionNum := ix("ix_region_num", "region", "num")
	dupA := ix("ix_sku_a", "sku")
	dupB := ix("ix_sku_b", "sku")
	uniqSku := ix("uq_sku_c", "sku")
	uniqSku.Unique = true
	uniqPrefix := ix("uq_region", "region")
	uniqPrefix.Unique = true
	partial := ix("ix_region_partial", "region")
	partial.Partial = true
	ft := ix("ft_region", "region")
	ft.Method = "FULLTEXT"
	idUnused := ix("ix_id_dup", "id")
	cold := ix("ix_cold", "qty")
	cold.Scans = ptr64(0)
	coldUnique := ix("uq_cold", "code")
	coldUnique.Unique, coldUnique.Scans = true, ptr64(0)
	prefOnly := IndexDef{Table: "lines", Name: "ix_a", Columns: []string{"a"}, Method: "BTREE", Scans: ptr64(3)}
	prefLong := IndexDef{Table: "lines", Name: "ix_a_b", Columns: []string{"a", "b"}, Method: "BTREE", Scans: ptr64(3)}
	other := IndexDef{Table: "orders", Name: "ix_region", Columns: []string{"region"}, Method: "BTREE", Scans: ptr64(0)}

	rep := &IndexReport{UsageAvailable: true, Indexes: []IndexDef{
		pk, region, regionNum, dupA, dupB, uniqSku, uniqPrefix, partial, ft, idUnused, cold, coldUnique, other, prefOnly, prefLong,
	}}
	got := AnalyzeIndexes(rep, "mysql")
	red := findings(got.Redundant)
	want := map[string]string{
		"ix_region": "uq_region", // same columns as a unique index beats a longer one
		"ix_sku_a":  "uq_sku_c",  // same columns as a unique index
		"ix_sku_b":  "uq_sku_c",
		"ix_id_dup": "PRIMARY", // duplicates the primary key
		"ix_a":      "ix_a_b",  // leftmost prefix of a longer index
	}
	if len(red) != len(want) {
		t.Fatalf("redundant = %v, want %v", red, want)
	}
	for k, v := range want {
		if red[k] != v {
			t.Errorf("%s covered by %q, want %q", k, red[k], v)
		}
	}
	unused := findings(got.Unused)
	if len(unused) != 2 {
		t.Fatalf("unused = %v", unused)
	}
	if _, ok := unused["ix_cold"]; !ok {
		t.Error("ix_cold should be unused")
	}
	if _, ok := unused["ix_region"]; !ok { // orders.ix_region: same name, other table
		t.Error("orders.ix_region should be unused")
	}
	for _, f := range got.Redundant {
		if f.Index.Name == "ix_region" && f.DropSQL != "DROP INDEX `ix_region` ON `items`;" {
			t.Errorf("drop = %q", f.DropSQL)
		}
	}
}

func TestAnalyzeIndexesTwinsAndPostgres(t *testing.T) {
	a := IndexDef{Schema: "public", Table: "t", Name: "b_idx", Columns: []string{"x"}, Method: "btree"}
	b := IndexDef{Schema: "public", Table: "t", Name: "a_idx", Columns: []string{"x"}, Method: "btree"}
	got := AnalyzeIndexes(&IndexReport{Indexes: []IndexDef{a, b}}, "postgres")
	if len(got.Redundant) != 1 || got.Redundant[0].Index.Name != "b_idx" || got.Redundant[0].CoveredBy != "a_idx" {
		t.Fatalf("twins = %+v", got.Redundant)
	}
	if got.Redundant[0].DropSQL != `DROP INDEX CONCURRENTLY "public"."b_idx";` {
		t.Errorf("drop = %q", got.Redundant[0].DropSQL)
	}
	if len(got.Unused) != 0 {
		t.Errorf("usage unavailable must report nothing unused: %+v", got.Unused)
	}
}

func TestExplainable(t *testing.T) {
	for _, bad := range []string{"SELECT 1; SELECT 2", "EXPLAIN SELECT 1", "analyze select 1", "", "-- nothing"} {
		if _, err := explainable(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
	if s, err := explainable("  SELECT 1 ;"); err != nil || s == "" {
		t.Errorf("single statement refused: %v", err)
	}
}
