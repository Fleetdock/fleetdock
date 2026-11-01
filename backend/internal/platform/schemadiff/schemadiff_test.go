package schemadiff

import (
	"strings"
	"testing"
)

func mysqlSchema(tables ...Table) *Schema {
	s := &Schema{Dialect: MySQL, Engine: "mysql", Tables: tables}
	s.Canonicalize()
	return s
}

func col(name, def string) Column { return Column{Name: name, Def: def} }

func TestHashIgnoresColumnOrderAndEngine(t *testing.T) {
	a := mysqlSchema(Table{Name: "t", Columns: []Column{col("a", "`a` int"), col("b", "`b` int")}})
	b := mysqlSchema(Table{Name: "t", Columns: []Column{col("b", "`b` int"), col("a", "`a` int")}})
	b.Engine = "mariadb"
	if a.Hash() != b.Hash() {
		t.Fatal("column order or engine changed the hash")
	}
	if a.Tables[0].Columns[0].Name != "a" {
		t.Fatal("Hash reordered the schema's columns")
	}
	c := mysqlSchema(Table{Name: "t", Columns: []Column{col("a", "`a` bigint"), col("b", "`b` int")}})
	if a.Hash() == c.Hash() {
		t.Fatal("a type change kept the hash")
	}
}

func TestDiffOps(t *testing.T) {
	a := mysqlSchema(
		Table{Name: "keep", Columns: []Column{col("id", "`id` int"), col("gone", "`gone` int"), col("v", "`v` int")},
			Indexes: []Object{{Name: "ix", Kind: "INDEX", Def: "KEY `ix` (`v`)"}}},
		Table{Name: "old", Columns: []Column{col("id", "`id` int")}},
	)
	b := mysqlSchema(
		Table{Name: "keep", Columns: []Column{col("id", "`id` int"), col("v", "`v` bigint"), col("new", "`new` int")},
			Indexes: []Object{{Name: "ix", Kind: "INDEX", Def: "KEY `ix` (`v`,`id`)"}}},
		Table{Name: "fresh", Columns: []Column{col("id", "`id` int")}},
	)
	got := map[string]string{}
	for _, c := range Diff(a, b) {
		got[c.Label()] = c.Op
	}
	want := map[string]string{
		"column keep.v": Changed, "column keep.new": Added, "column keep.gone": Removed,
		"index keep.ix": Changed, "table fresh": Added, "table old": Removed,
	}
	if len(got) != len(want) {
		t.Fatalf("diff = %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q", k, got[k], v)
		}
	}
}

func TestMigrateMySQL(t *testing.T) {
	from := mysqlSchema(
		Table{Name: "t", Columns: []Column{col("id", "`id` int NOT NULL"), col("gone", "`gone` int")},
			Constraints: []Object{{Name: "chk", Kind: "CHECK", Def: "CONSTRAINT `chk` CHECK ((`id` > 0))"}}},
		Table{Name: "old", Columns: []Column{col("id", "`id` int")}},
	)
	to := mysqlSchema(
		Table{Name: "t", Columns: []Column{col("id", "`id` int NOT NULL"), col("n", "`n` int")},
			Indexes:     []Object{{Name: "PRIMARY", Kind: "PRIMARY", Def: "PRIMARY KEY (`id`)"}},
			Constraints: []Object{{Name: "chk", Kind: "CHECK", Def: "CONSTRAINT `chk` CHECK ((`id` > 1))"}}},
	)
	to.Routines = []Object{{Name: "p", Kind: "PROCEDURE", Def: "CREATE PROCEDURE `p`() BEGIN SELECT 1; END"}}
	s := Migrate(from, to)
	for _, want := range []string{
		"SET FOREIGN_KEY_CHECKS = 0;",
		"ALTER TABLE `t` DROP CHECK `chk`;",
		"ALTER TABLE `t` ADD COLUMN `n` int AFTER `id`;",
		"-- REVIEW (drops the column's data): ALTER TABLE `t` DROP COLUMN `gone`;",
		"ALTER TABLE `t` ADD PRIMARY KEY (`id`);",
		"ALTER TABLE `t` ADD CONSTRAINT `chk` CHECK ((`id` > 1));",
		"-- REVIEW (drops the table and its rows): DROP TABLE `old`;",
		"DELIMITER $$\nCREATE PROCEDURE `p`() BEGIN SELECT 1; END $$\nDELIMITER ;",
	} {
		if !strings.Contains(s.SQL, want) {
			t.Errorf("script lacks %q:\n%s", want, s.SQL)
		}
	}
	if s.Review != 2 {
		t.Errorf("review = %d", s.Review)
	}
	from.Engine = "mariadb"
	if !strings.Contains(Migrate(from, to).SQL, "DROP CONSTRAINT `chk`") {
		t.Error("MariaDB checks are dropped with DROP CONSTRAINT")
	}
}

func TestPGColumnAndEnum(t *testing.T) {
	c := Column{Name: "id", Type: "integer", Identity: "ALWAYS", NotNull: true}
	if got := PGColumnDef(c); got != `"id" integer GENERATED ALWAYS AS IDENTITY NOT NULL` {
		t.Fatalf("def = %s", got)
	}
	mk := func(cols []Column, values ...string) *Schema {
		for i := range cols {
			cols[i].Def = PGColumnDef(cols[i])
		}
		s := &Schema{Dialect: Postgres, Tables: []Table{{Schema: "public", Name: "t", Columns: cols}},
			Types: []Object{{Schema: "public", Name: "mood", Kind: "ENUM", Values: values, Def: strings.Join(values, ",")}}}
		s.Canonicalize()
		return s
	}
	from := mk([]Column{{Name: "id", Type: "integer"}, {Name: "s", Type: "text", Default: "'a'::text"}}, "sad", "happy")
	to := mk([]Column{{Name: "id", Type: "bigint", NotNull: true}, {Name: "s", Type: "text"}}, "meh", "sad", "ok", "happy")
	sql := Migrate(from, to).SQL
	for _, want := range []string{
		`ALTER TYPE "public"."mood" ADD VALUE 'meh' BEFORE 'sad';`,
		`ALTER TYPE "public"."mood" ADD VALUE 'ok' AFTER 'sad';`,
		`ALTER TABLE "public"."t" ALTER COLUMN "id" TYPE bigint USING "id"::bigint;`,
		`ALTER TABLE "public"."t" ALTER COLUMN "id" SET NOT NULL;`,
		`ALTER TABLE "public"."t" ALTER COLUMN "s" DROP DEFAULT;`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("script lacks %q:\n%s", want, sql)
		}
	}
	// Removing a label cannot be done in place.
	if s := Migrate(to, from); !strings.Contains(s.SQL, "-- manual: type") || s.Review != 1 {
		t.Errorf("enum shrink:\n%s", s.SQL)
	}
}

func TestPGNewSerialTable(t *testing.T) {
	cols := []Column{{Name: "id", Type: "bigint", NotNull: true, Default: "nextval('public.t_id_seq'::regclass)"}}
	cols[0].Def = PGColumnDef(cols[0])
	to := &Schema{Dialect: Postgres, Tables: []Table{{Schema: "public", Name: "t", Columns: cols,
		Constraints: []Object{{Name: "t_pkey", Kind: "p", Def: "PRIMARY KEY (id)"}}}}}
	sql := Migrate(&Schema{Dialect: Postgres}, to).SQL
	seq := strings.Index(sql, "CREATE SEQUENCE IF NOT EXISTS public.t_id_seq;")
	tbl := strings.Index(sql, `CREATE TABLE "public"."t"`)
	pk := strings.Index(sql, `ADD CONSTRAINT "t_pkey" PRIMARY KEY (id);`)
	own := strings.Index(sql, `ALTER SEQUENCE public.t_id_seq OWNED BY "public"."t"."id";`)
	if seq < 0 || tbl < seq || pk < tbl || own < pk {
		t.Fatalf("order wrong:\n%s", sql)
	}
}
