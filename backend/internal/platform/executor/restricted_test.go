package executor

import (
	"context"
	"io"
	"strings"
	"testing"
)

// fakeTool makes toolOutput answer "--version" with version and "--help"
// with help.
func fakeTool(t *testing.T, version, help string) {
	t.Helper()
	orig := toolOutput
	t.Cleanup(func() { toolOutput = orig })
	toolOutput = func(_ context.Context, _ string, args ...string) (string, error) {
		if len(args) == 1 && args[0] == "--help" {
			return help, nil
		}
		return version, nil
	}
}

func TestParsePSQLVersion(t *testing.T) {
	cases := map[string][2]int{
		"psql (PostgreSQL) 18.1\n":                 {18, 1},
		"psql (PostgreSQL) 17.6 (Debian 17.6-1)\n": {17, 6},
		"psql (PostgreSQL) 14.24 (Homebrew)\n":     {14, 24},
		"psql (PostgreSQL) 19beta1\n":              {19, 0},
	}
	for in, want := range cases {
		major, minor, ok := parsePSQLVersion(in)
		if !ok || major != want[0] || minor != want[1] {
			t.Errorf("parsePSQLVersion(%q) = %d.%d %v, want %d.%d", in, major, minor, ok, want[0], want[1])
		}
	}
	if _, _, ok := parsePSQLVersion("mysql  Ver 8.4.3"); ok {
		t.Error("parsed a version out of a non-psql banner")
	}
}

func TestPSQLRestrictSupported(t *testing.T) {
	for _, c := range []struct {
		major, minor int
		ok           bool
	}{
		{18, 0, true}, {19, 0, true},
		{17, 6, true}, {17, 5, false},
		{16, 10, true}, {16, 9, false},
		{15, 14, true}, {15, 13, false},
		{14, 19, true}, {14, 18, false},
		{13, 22, true}, {13, 21, false},
		{12, 99, false},
	} {
		need, ok := psqlRestrictSupported(c.major, c.minor)
		if ok != c.ok {
			t.Errorf("%d.%d: supported = %v, want %v", c.major, c.minor, ok, c.ok)
		}
		if !ok && need == "" {
			t.Errorf("%d.%d: no upgrade target given", c.major, c.minor)
		}
	}
}

func TestGuardRestoreWrapsPSQLInRestrictedMode(t *testing.T) {
	fakeTool(t, "psql (PostgreSQL) 18.1\n", "")
	body := "CREATE TABLE t (a int);\n\\! id\n"
	r, err := guardRestore(context.Background(), "postgres", "psql", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(r)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	first, last := lines[0], lines[len(lines)-1]
	if !strings.HasPrefix(first, `\restrict `) || !strings.HasPrefix(last, `\unrestrict `) {
		t.Fatalf("stream not wrapped: first %q, last %q", first, last)
	}
	key := strings.TrimPrefix(first, `\restrict `)
	if len(key) < 32 || strings.TrimPrefix(last, `\unrestrict `) != key {
		t.Errorf("restrict key %q / unrestrict %q", key, last)
	}
	if !strings.Contains(string(out), body) {
		t.Error("stream body altered")
	}

	r2, _ := guardRestore(context.Background(), "postgres", "psql", strings.NewReader(body))
	out2, _ := io.ReadAll(r2)
	if strings.SplitN(string(out2), "\n", 2)[0] == first {
		t.Error("restrict key repeated between restores")
	}
}

func TestGuardRestoreRefusesOldPSQL(t *testing.T) {
	fakeTool(t, "psql (PostgreSQL) 15.3\n", "")
	_, err := guardRestore(context.Background(), "postgres", "/usr/bin/psql", strings.NewReader("SELECT 1;"))
	if err == nil || !strings.Contains(err.Error(), "upgrade it to 15.14") {
		t.Fatalf("err = %v, want a refusal naming 15.14", err)
	}
}

func TestGuardRestoreMariaDBNeedsSandbox(t *testing.T) {
	fakeTool(t, "mariadb from 11.4.5-MariaDB, client 15.2 for Linux (x86_64)",
		"  --safe-updates      Only allow UPDATE and DELETE that uses keys.\n  --sandbox           Disallow commands that access the file system\n")
	if _, err := guardRestore(context.Background(), "mysql", "mariadb", strings.NewReader("SELECT 1;")); err != nil {
		t.Fatalf("sandbox-capable client refused: %v", err)
	}

	fakeTool(t, "mysql  Ver 8.4.3 for macos15 on arm64 (Homebrew)",
		"  --safe-updates      Only allow UPDATE and DELETE that uses keys.\n  --show-warnings     Show warnings after every statement.\n")
	_, err := guardRestore(context.Background(), "mysql", "mysql", strings.NewReader("SELECT 1;"))
	if err == nil || !strings.Contains(err.Error(), "--sandbox") {
		t.Fatalf("err = %v, want a refusal about --sandbox", err)
	}
}

func TestGuardRestoreUnknownEngine(t *testing.T) {
	if _, err := guardRestore(context.Background(), "clickhouse", "x", strings.NewReader("")); err == nil {
		t.Fatal("unknown engine accepted")
	}
}
