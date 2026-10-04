package executor

import (
	"io"
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
