#!/bin/sh
# Fleetdock agent installer
# Usage:
#   Install and enroll:
#     curl -sSL <control-plane>/install.sh | sudo env FLEETDOCK_TOKEN=<registration-token> sh
#   Update an enrolled agent to the control plane's version (keeps its identity):
#     curl -sSL <control-plane>/install.sh | sudo sh
set -eu

FLEETDOCK_URL="${FLEETDOCK_URL:-%s}"
FLEETDOCK_TOKEN="${FLEETDOCK_TOKEN:-}"
STATE_DIR=/var/lib/fleetdock-agent
BIN=/usr/local/bin/fleetdock-agent

if [ "$(id -u)" != "0" ]; then
  echo "error: run as root (or with sudo)." >&2
  exit 1
fi

MODE=install
if [ -z "$FLEETDOCK_TOKEN" ]; then
  if [ -s "$STATE_DIR/state.json" ] && [ -f /etc/systemd/system/fleetdock-agent.service ]; then
    MODE=update
  else
    echo "error: FLEETDOCK_TOKEN is required. Generate a registration token in the dashboard (Servers -> Connect server)." >&2
    exit 1
  fi
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

# Download next to the target and rename into place: the rename is atomic and
# works while the old binary is running (writing over it would fail with
# "text file busy"). The checksum guards against a truncated or altered
# download.
echo "==> Downloading agent (${OS}/${ARCH})"
mkdir -p /usr/local/bin
TMP_BIN="$(mktemp /usr/local/bin/.fleetdock-agent.XXXXXX)"
trap 'rm -f "$TMP_BIN"' EXIT
curl -fsSL "${FLEETDOCK_URL}/agent/v1/binary/${OS}/${ARCH}" -o "$TMP_BIN"
WANT_SUM="$(curl -fsSL "${FLEETDOCK_URL}/agent/v1/binary/${OS}/${ARCH}/sha256" | tr -d '[:space:]')"
if command -v sha256sum >/dev/null 2>&1; then
  GOT_SUM="$(sha256sum "$TMP_BIN" | cut -d' ' -f1)"
else
  GOT_SUM="$(openssl dgst -sha256 -r "$TMP_BIN" | cut -d' ' -f1)"
fi
if [ -z "$WANT_SUM" ] || [ "$GOT_SUM" != "$WANT_SUM" ]; then
  echo "error: the downloaded agent does not match its checksum (got ${GOT_SUM:-none}, expected ${WANT_SUM:-none})." >&2
  exit 1
fi
chmod 755 "$TMP_BIN"
mv -f "$TMP_BIN" "$BIN"
trap - EXIT
NEW_VERSION="$("$BIN" version 2>/dev/null || echo unknown)"

if [ "$MODE" = update ]; then
  systemctl restart fleetdock-agent
  echo ""
  echo "Fleetdock agent updated to ${NEW_VERSION} and restarted."
  echo "The dashboard shows the new version after the next heartbeat (~15 seconds)."
  exit 0
fi

echo "==> Installing database client tools (for backups/restores)"
if command -v apt-get >/dev/null 2>&1; then
  if apt-get update -qq; then
    apt-get install -y -qq mariadb-client postgresql-client >/dev/null 2>&1 || true
  fi
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

mkdir -p /etc/fleetdock-agent "$STATE_DIR"
# A registration token means "enroll now": drop any saved agent identity so a
# re-run (for example with a re-enrollment token after rotating this server's
# agent token) enrolls again instead of reusing the old, revoked credentials.
rm -f "$STATE_DIR/state.json"
cat > /etc/fleetdock-agent/agent.env <<EOF
FLEETDOCK_URL=${FLEETDOCK_URL}
FLEETDOCK_TOKEN=${FLEETDOCK_TOKEN}
FLEETDOCK_STATE_DIR=${STATE_DIR}
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
# The agent runs as root because it drives Docker and the database client
# tools; this sandbox narrows what a compromised agent process can change.
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=read-only
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
ProtectClock=true
ProtectHostname=true
RestrictSUIDSGID=true
RestrictRealtime=true
RestrictNamespaces=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now fleetdock-agent

echo ""
echo "Fleetdock agent ${NEW_VERSION} installed and started."
echo "The server will appear in your dashboard within ~30 seconds."
