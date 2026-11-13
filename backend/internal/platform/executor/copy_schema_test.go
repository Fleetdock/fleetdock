package executor

import (
	"io"
	"strings"
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

func TestEditForeignKeysLeavesRowsAlone(t *testing.T) {
	dump := strings.Join([]string{
		"COPY sales.notes (body) FROM stdin;",
		"    ADD CONSTRAINT x FOREIGN KEY (a) REFERENCES b(c);",
		`\.`,
		"ALTER TABLE ONLY sales.customers",
		"    ADD CONSTRAINT customers_country_fkey FOREIGN KEY (country) REFERENCES ref.country(code);",
		"ALTER TABLE ONLY sales.orders",
		"    ADD CONSTRAINT orders_pkey PRIMARY KEY (id);",
		"",
	}, "\n")
	var seen []string
	r := editForeignKeys(strings.NewReader(dump), func(table, name, line string) string {
		seen = append(seen, table+"/"+name)
		return strings.TrimSuffix(line, ";\n") + " NOT VALID;\n"
	})
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(seen, ",") != "sales.customers/customers_country_fkey" {
		t.Errorf("edited %v", seen)
	}
	if !strings.Contains(string(out), "REFERENCES ref.country(code) NOT VALID;") {
		t.Errorf("foreign key not edited:\n%s", out)
	}
	if !strings.Contains(string(out), "    ADD CONSTRAINT x FOREIGN KEY (a) REFERENCES b(c);\n") {
		t.Errorf("a row was edited:\n%s", out)
	}
}

func TestPartialDumpArgsQuotesSchemas(t *testing.T) {
	_, args, _ := pgPartialArgs()
	got := strings.Join(args, " ")
	for _, want := range []string{`--schema="Sales"`, "--no-owner"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q lack %q", got, want)
		}
	}
	if strings.Contains(got, "--clean") {
		t.Errorf("a partial dump must not drop anything: %q", got)
	}
}

func pgPartialArgs() ([]string, []string, []string) {
	return (&engine.Postgres{}).PartialDumpArgs(engine.ConnParams{Host: "db", Port: 5432, User: "u"}, "app",
		engine.PartialDump{Schema: "Sales", SkipDataOf: []string{"ref"}})
}
