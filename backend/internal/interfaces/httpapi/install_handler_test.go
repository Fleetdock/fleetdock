package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstallHandlerBinarySum(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fleetdock-agent-linux-amd64")
	if err := os.WriteFile(bin, []byte("agent v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := NewInstallHandler("https://fleetdock.example.com", dir)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /agent/v1/binary/{os}/{arch}/sha256", h.BinarySum)

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}
	want := func(body string) string {
		s := sha256.Sum256([]byte(body))
		return hex.EncodeToString(s[:])
	}

	rec := get("/agent/v1/binary/linux/amd64/sha256")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != want("agent v1") {
		t.Fatalf("sum = %d %q", rec.Code, rec.Body.String())
	}

	// A replaced binary gets a fresh checksum, not the cached one.
	if err := os.WriteFile(bin, []byte("agent v2 (longer)"), 0o755); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(bin, later, later); err != nil {
		t.Fatal(err)
	}
	if rec := get("/agent/v1/binary/linux/amd64/sha256"); strings.TrimSpace(rec.Body.String()) != want("agent v2 (longer)") {
		t.Fatalf("stale sum after replacing the binary: %q", rec.Body.String())
	}

	for _, path := range []string{"/agent/v1/binary/linux/arm64/sha256", "/agent/v1/binary/linux/..%2f/sha256"} {
		if rec := get(path); rec.Code == http.StatusOK {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}

func TestInstallScriptHasUpdateModeAndChecksum(t *testing.T) {
	for _, want := range []string{"MODE=update", "/sha256", "mv -f \"$TMP_BIN\" \"$BIN\""} {
		if !strings.Contains(installScriptTemplate, want) {
			t.Errorf("install script lacks %q", want)
		}
	}
	// The template goes through Fprintf with one argument: a stray verb
	// would corrupt the script.
	if n := strings.Count(installScriptTemplate, "%"); n != 1 {
		t.Errorf("install script has %d %% signs, want exactly the URL's %%s", n)
	}
}
