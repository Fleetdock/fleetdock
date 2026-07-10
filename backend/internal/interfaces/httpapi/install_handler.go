package httpapi

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// InstallHandler serves the one-command agent install script and the
// cross-compiled agent binaries bundled with the control plane.
type InstallHandler struct {
	publicURL string
	binDir    string
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

func sanitizeComponent(s string) string {
	s = strings.ToLower(s)
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return s
}
