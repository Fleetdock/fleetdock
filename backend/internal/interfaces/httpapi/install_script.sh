#!/bin/sh
# Fleetdock agent installer
# Usage:
#   curl -sSL <control-plane>/install.sh | FLEETDOCK_URL=<control-plane> FLEETDOCK_TOKEN=<registration-token> sh
set -eu

FLEETDOCK_URL="${FLEETDOCK_URL:-%s}"
FLEETDOCK_TOKEN="${FLEETDOCK_TOKEN:-}"

if [ -z "$FLEETDOCK_TOKEN" ]; then
  echo "error: FLEETDOCK_TOKEN is required. Generate a registration token in the dashboard (Servers -> Connect server)." >&2
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
curl -fsSL "${FLEETDOCK_URL}/agent/v1/binary/${OS}/${ARCH}" -o /usr/local/bin/fleetdock-agent
chmod +x /usr/local/bin/fleetdock-agent

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

mkdir -p /etc/fleetdock-agent /var/lib/fleetdock-agent
cat > /etc/fleetdock-agent/agent.env <<EOF
FLEETDOCK_URL=${FLEETDOCK_URL}
FLEETDOCK_TOKEN=${FLEETDOCK_TOKEN}
FLEETDOCK_STATE_DIR=/var/lib/fleetdock-agent
EOF
chmod 600 /etc/fleetdock-agent/agent.env

echo "==> Installing systemd service"
cat > /etc/systemd/system/fleetdock-agent.service <<'EOF'
[Unit]
Description=Fleetdock agent
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/fleetdock-agent/agent.env
ExecStart=/usr/local/bin/fleetdock-agent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now fleetdock-agent

echo ""
echo "Fleetdock agent installed and started."
echo "The server will appear in your dashboard within ~30 seconds."
