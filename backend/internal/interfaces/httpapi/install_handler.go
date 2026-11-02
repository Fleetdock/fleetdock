package httpapi

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// InstallHandler serves the one-command agent install script and the
// cross-compiled agent binaries bundled with the control plane.
type InstallHandler struct {
	publicURL string
	binDir    string

	mu   sync.Mutex
	sums map[string]fileSum // binary path -> checksum, recomputed when the file changes
}

type fileSum struct {
	mod  time.Time
	size int64
	hex  string
}

//go:embed install_script.sh
var installScriptTemplate string

// NewInstallHandler builds the install handler.
func NewInstallHandler(publicURL, binDir string) *InstallHandler {
	return &InstallHandler{publicURL: publicURL, binDir: binDir}
}

// Script handles GET /install.sh (public).
func (h *InstallHandler) Script(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = fmt.Fprintf(w, installScriptTemplate, h.publicURL)
}

// Binary handles GET /agent/v1/binary/{os}/{arch} (public: binaries are not
// secret; enrollment still requires a registration token).
func (h *InstallHandler) Binary(w http.ResponseWriter, r *http.Request) {
	osName := sanitizeComponent(r.PathValue("os"))
	arch := sanitizeComponent(r.PathValue("arch"))
	if osName == "" || arch == "" {
		http.Error(w, "invalid platform", http.StatusBadRequest)
		return
	}
	path := filepath.Join(h.binDir, fmt.Sprintf("fleetdock-agent-%s-%s", osName, arch))
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "agent binary not available for this platform", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, path)
}

// BinarySum handles GET /agent/v1/binary/{os}/{arch}/sha256: the binary's
// SHA-256 as hex, which install.sh checks the download against.
func (h *InstallHandler) BinarySum(w http.ResponseWriter, r *http.Request) {
	osName := sanitizeComponent(r.PathValue("os"))
	arch := sanitizeComponent(r.PathValue("arch"))
	if osName == "" || arch == "" {
		http.Error(w, "invalid platform", http.StatusBadRequest)
		return
	}
	sum, err := h.checksum(filepath.Join(h.binDir, fmt.Sprintf("fleetdock-agent-%s-%s", osName, arch)))
	if err != nil {
		http.Error(w, "agent binary not available for this platform", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintln(w, sum)
}

func (h *InstallHandler) checksum(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.sums[path]; ok && c.mod.Equal(st.ModTime()) && c.size == st.Size() {
		return c.hex, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	d := sha256.New()
	if _, err := io.Copy(d, f); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(d.Sum(nil))
	if h.sums == nil {
		h.sums = map[string]fileSum{}
	}
	h.sums[path] = fileSum{mod: st.ModTime(), size: st.Size(), hex: sum}
	return sum, nil
}

func sanitizeComponent(s string) string {
	s = strings.ToLower(s)
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return s
}
