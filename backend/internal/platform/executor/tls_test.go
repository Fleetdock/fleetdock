package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTLSArgs(t *testing.T) {
	pg := strings.Join(tlsArgs("postgres"), " ")
	if !strings.HasPrefix(pg, "postgres -c ssl=on ") || !strings.Contains(pg, "ssl_key_file="+tlsMountPath+"/server.key") {
		t.Errorf("postgres args = %q", pg)
	}
	for _, eng := range []string{"mysql", "mariadb"} {
		got := tlsArgs(eng)
		if len(got) == 0 || !strings.HasPrefix(got[0], "--ssl-cert=") {
			t.Errorf("%s args = %v", eng, got)
		}
	}
}

func TestWriteTLSFiles(t *testing.T) {
	t.Setenv("FLEETDOCK_STATE_DIR", t.TempDir())
	dir, err := writeTLSFiles("dbm-x", &TLSFiles{CertPEM: "CERT", KeyPEM: "KEY", CAPEM: "CA"})
	if err != nil {
		t.Fatalf("writeTLSFiles: %v", err)
	}
	for name, want := range map[string]os.FileMode{"server.crt": 0o644, "server.key": 0o600, "ca.crt": 0o644} {
		st, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if st.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", name, st.Mode().Perm(), want)
		}
	}
	if _, err := writeTLSFiles("dbm-x", &TLSFiles{}); err == nil {
		t.Error("an empty certificate was accepted")
	}
}
