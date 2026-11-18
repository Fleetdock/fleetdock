package executor

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPGDumpCompatDropsTransactionTimeout(t *testing.T) {
	long := strings.Repeat("x", 200<<10) // longer than the read buffer
	in := "SET statement_timeout = 0;\nSET transaction_timeout = 0;\nCREATE TABLE t (a text);\n" + long + "\nSELECT 1;"
	out, err := io.ReadAll(newPGDumpCompat(strings.NewReader(in)))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "transaction_timeout") {
		t.Error("transaction_timeout line was not dropped")
	}
	want := "SET statement_timeout = 0;\nCREATE TABLE t (a text);\n" + long + "\nSELECT 1;"
	if got != want {
		t.Errorf("stream altered beyond the dropped line (len %d, want %d)", len(got), len(want))
	}
}

func TestPGDumpCompatDropsRestrictOutsideCopyOnly(t *testing.T) {
	in := "\\restrict abc123\n" +
		"SET transaction_timeout = 0;\n" +
		"COPY public.t (a) FROM stdin;\n" +
		"SET transaction_timeout = 0;\n" +
		"\\restrict notakey\n" +
		"\\.\n" +
		"SELECT 1;\n" +
		"\\unrestrict abc123\n"
	out, err := io.ReadAll(newPGDumpCompat(strings.NewReader(in)))
	if err != nil {
		t.Fatal(err)
	}
	want := "COPY public.t (a) FROM stdin;\n" +
		"SET transaction_timeout = 0;\n" +
		"\\restrict notakey\n" +
		"\\.\n" +
		"SELECT 1;\n"
	if got := string(out); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestFirstLinePrefersTheError(t *testing.T) {
	cases := map[string]string{
		"WARNING: option --ssl-verify-server-cert is disabled\nERROR 1227 (42000) at line 1: Access denied": "ERROR 1227 (42000) at line 1: Access denied",
		"mysqldump: [Warning] Using a password on the command line interface can be insecure.":              "command failed",
		"psql:<stdin>:3: ERROR:  permission denied to create role":                                          "psql:<stdin>:3: ERROR:  permission denied to create role",
		"something odd happened": "something odd happened",
	}
	for in, want := range cases {
		if got := firstLine(in); got != want {
			t.Errorf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEngineContainerDataPath(t *testing.T) {
	for _, c := range []struct{ eng, version, want string }{
		{"postgres", "16", "/var/lib/postgresql/data"},
		{"postgres", "17.2", "/var/lib/postgresql/data"},
		{"postgres", "18", "/var/lib/postgresql"}, // the 18 image refuses .../data
		{"postgres", "", "/var/lib/postgresql/data"},
		{"mysql", "8.4", "/var/lib/mysql"},
		{"mariadb", "11.8", "/var/lib/mysql"},
	} {
		if got := engineContainerFor(c.eng, c.version).dataPath; got != c.want {
			t.Errorf("%s %s: %s, want %s", c.eng, c.version, got, c.want)
		}
	}
}

func TestImageRef(t *testing.T) {
	cases := []struct {
		eng  string
		spec ProvisionSpec
		want string
	}{
		{"mariadb", ProvisionSpec{Version: "11.4"}, "mariadb:11.4"},
		{"postgres", ProvisionSpec{Image: "postgres", Version: "17"}, "postgres:17"},
		{"postgres", ProvisionSpec{Image: "pgvector/pgvector", Version: "17", Tag: "pg17"}, "pgvector/pgvector:pg17"},
	}
	for _, c := range cases {
		if got := imageRef(c.eng, &c.spec); got != c.want {
			t.Errorf("imageRef(%q, %+v) = %q, want %q", c.eng, c.spec, got, c.want)
		}
	}
}

func TestCrashed(t *testing.T) {
	cases := map[string]bool{
		"true false 0\n": false, // starting normally
		"false false 0":  true,  // stopped
		"true true 2":    true,  // being restarted
		"true false 3":   true,  // came back after crashing: a crash loop
		"":               false, // inspect output not understood: keep waiting
	}
	for in, want := range cases {
		if got := crashed(in); got != want {
			t.Errorf("crashed(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNewestPGToolPicksHighestMajor(t *testing.T) {
	root := t.TempDir()
	for _, v := range []string{"9.6", "16", "18", "17"} {
		dir := filepath.Join(root, "lib", v, "bin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pg_dump"), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A newer major without the tool installed must not win.
	if err := os.MkdirAll(filepath.Join(root, "lib", "19", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := pgToolDirs
	pgToolDirs = []string{filepath.Join(root, "lib", "*", "bin")}
	t.Cleanup(func() { pgToolDirs = old })

	if got, want := newestPGTool("pg_dump"), filepath.Join(root, "lib", "18", "bin", "pg_dump"); got != want {
		t.Errorf("newestPGTool(pg_dump) = %q, want %q", got, want)
	}
	if got := newestPGTool("psql"); got != "" {
		t.Errorf("newestPGTool(psql) = %q, want none", got)
	}
	if got := newestPGTool("mariadb-dump"); got != "" {
		t.Errorf("newestPGTool(mariadb-dump) = %q, want none", got)
	}
}

func TestPickLogLine(t *testing.T) {
	logs := "Processing configuration file '/etc/clickhouse-server/config.xml'.\n" +
		"2026.11.15 00:00:01.000 <Error> Application: DB::Exception: Listen [::]:8123 failed: Address already in use\n" +
		"Logging trace to /var/log/clickhouse-server/clickhouse-server.log\n\n"
	if got := pickLogLine(logs); !strings.Contains(got, "Address already in use") {
		t.Errorf("pickLogLine = %q, want the error line", got)
	}
	if got := pickLogLine("starting\nstill starting\n"); got != "still starting" {
		t.Errorf("without an error line, the last line: %q", got)
	}
	if got := pickLogLine(""); got != "no log output" {
		t.Errorf("empty logs: %q", got)
	}
}

// TestProvisionContainerRefusesClickHouseTLS: the certificate arguments are
// PostgreSQL's and MySQL's, so a ClickHouse container is never started with
// them (the API refuses it first).
func TestProvisionContainerRefusesClickHouseTLS(t *testing.T) {
	spec := &ProvisionSpec{ContainerName: "ch", TLS: &TLSFiles{CertPEM: "c", KeyPEM: "k"}}
	_, err := provisionContainer(context.Background(), "clickhouse", spec, "clickhouse:25.8", true, NopSink{})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("err = %v, want a refusal", err)
	}
}
