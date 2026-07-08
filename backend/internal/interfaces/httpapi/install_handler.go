package httpapi

import (
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

// NewInstallHandler builds the install handler.
func NewInstallHandler(publicURL, binDir string) *InstallHandler {
	return &InstallHandler{publicURL: publicURL, binDir: binDir}
}

// Script handles GET /install.sh (public).
func (h *InstallHandler) Script(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = fmt.Fprintf(w, installScript, h.publicURL)
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
	path := filepath.Join(h.binDir, fmt.Sprintf("db-manager-agent-%s-%s", osName, arch))
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

// installScript is the agent installer. %s is the control plane public URL
// (a default; the MDCP_URL env var always wins).
const installScript = `#!/bin/sh
# db-manager agent installer
# Usage:
#   curl -sSL <control-plane>/install.sh | MDCP_URL=<control-plane> MDCP_TOKEN=<registration-token> sh
set -eu

MDCP_URL="${MDCP_URL:-%s}"
MDCP_TOKEN="${MDCP_TOKEN:-}"

if [ -z "$MDCP_TOKEN" ]; then
  echo "error: MDCP_TOKEN is required. Generate a registration token in the dashboard (Servers -> Connect server)." >&2
  exit 1
fi
if [ "$(id -u)" != "0" ]; then
  echo "error: run as root (or with sudo)." >&2
  exit 1
fi

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "error: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac
if [ "$OS" != "linux" ]; then
  echo "error: only linux servers are supported" >&2
  exit 1
fi

echo "==> Downloading agent (${OS}/${ARCH})"
curl -fsSL "${MDCP_URL}/agent/v1/binary/${OS}/${ARCH}" -o /usr/local/bin/db-manager-agent
chmod +x /usr/local/bin/db-manager-agent

echo "==> Installing database client tools (for backups/restores)"
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -qq && apt-get install -y -qq mariadb-client postgresql-client >/dev/null 2>&1 || true
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y -q mariadb postgresql >/dev/null 2>&1 || true
elif command -v yum >/dev/null 2>&1; then
  yum install -y -q mariadb postgresql >/dev/null 2>&1 || true
elif command -v apk >/dev/null 2>&1; then
  apk add --no-cache mariadb-client postgresql-client >/dev/null 2>&1 || true
fi

echo "==> Ensuring Docker is installed (for provisioning database instances)"
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sh >/dev/null 2>&1 || \
    echo "warning: could not install Docker automatically; provisioning will be unavailable until Docker is installed" >&2
fi
if command -v systemctl >/dev/null 2>&1 && command -v docker >/dev/null 2>&1; then
  systemctl enable --now docker >/dev/null 2>&1 || true
fi

mkdir -p /etc/db-manager-agent /var/lib/db-manager-agent
cat > /etc/db-manager-agent/agent.env <<EOF
MDCP_URL=${MDCP_URL}
MDCP_TOKEN=${MDCP_TOKEN}
MDCP_STATE_DIR=/var/lib/db-manager-agent
EOF
chmod 600 /etc/db-manager-agent/agent.env

echo "==> Installing systemd service"
cat > /etc/systemd/system/db-manager-agent.service <<'EOF'
[Unit]
Description=db-manager agent
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/db-manager-agent/agent.env
ExecStart=/usr/local/bin/db-manager-agent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now db-manager-agent

echo ""
echo "db-manager agent installed and started."
echo "The server will appear in your dashboard within ~30 seconds."
`
