#!/bin/sh
# Fleetdock control-plane installer.
#
#   curl -sSL https://fleetdock.dev/install.sh | sh
#   curl -sSL https://fleetdock.dev/install.sh | sh -s -- --domain db.example.com
#
# Not to be confused with the *agent* installer, which your own control plane
# serves at https://<your-control-plane>/install.sh and which you run on each
# database server.
#
# Installs Docker if missing, writes /opt/fleetdock/{docker-compose.yml,.env},
# starts the stack behind Caddy with automatic HTTPS, and prints the dashboard
# URL and bootstrap credentials.
#
# Linux is the deployment target. macOS is supported for local evaluation: it
# needs Docker Desktop already running, installs under ~/.fleetdock, needs no
# root, and serves plain HTTP on localhost.
set -eu

FLEETDOCK_REPO="${FLEETDOCK_REPO:-fleetdock/fleetdock}"
# Where docker-compose.yml and the fleetdock CLI come from. Defaults to the
# published repo, but may point at a local checkout — which is what makes this
# installable from a fork, from an air-gapped mirror, or in a test VM before
# anything has been published.
FLEETDOCK_SOURCE="${FLEETDOCK_SOURCE:-}"
BUILD_LOCALLY=""
FLEETDOCK_DIR="${FLEETDOCK_DIR:-/opt/fleetdock}"
FLEETDOCK_RELEASE_TAG="${FLEETDOCK_RELEASE_TAG:-latest}"
FLEETDOCK_DOMAIN="${FLEETDOCK_DOMAIN:-}"
FLEETDOCK_ADMIN_EMAIL="${FLEETDOCK_ADMIN_EMAIL:-admin@example.com}"
WITH_GATEWAY=""
NO_TLS=""
COMPOSE_REF="main"

# --- output helpers -----------------------------------------------------------

step() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

# --- argument parsing ---------------------------------------------------------

usage() {
  cat <<'EOF'
Usage: install.sh [options]

  --domain <host>     Hostname for the dashboard. Gets an automatic TLS
                      certificate. Without it, an <ip>.sslip.io name is used so
                      HTTPS still works with no DNS setup.
  --admin-email <e>   Bootstrap admin account (default admin@example.com).
  --dir <path>        Install directory (default /opt/fleetdock; ~/.fleetdock
                      on macOS).
  --tag <tag>         Image tag to run (default latest).
  --with-gateway      Also start the external database access gateway. Adds a
                      50-port range; off by default.
  --no-tls            Serve plain HTTP on the host's IP. No certificate.
  --source <path|url> Take docker-compose.yml and the fleetdock CLI from here
                      instead of the published repo. A local checkout works.
  --build             Build the application image from --source instead of
                      pulling it. Requires --source to be a checkout.
  -h, --help          This message.

On macOS this installs a local evaluation stack on http://localhost: no TLS, no
remote agent enrolment, and no external database access. Pass your LAN address as
--domain to let agents on the same network enrol.

Every option also reads from the matching FLEETDOCK_* environment variable, so
`curl -sSL … | FLEETDOCK_DOMAIN=db.example.com sh` works too.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --domain) FLEETDOCK_DOMAIN="${2:?--domain needs a value}"; shift 2 ;;
    --admin-email) FLEETDOCK_ADMIN_EMAIL="${2:?--admin-email needs a value}"; shift 2 ;;
    --dir) FLEETDOCK_DIR="${2:?--dir needs a value}"; FLEETDOCK_DIR_SET=1; shift 2 ;;
    --tag) FLEETDOCK_RELEASE_TAG="${2:?--tag needs a value}"; shift 2 ;;
    --with-gateway) WITH_GATEWAY=1; shift ;;
    --no-tls) NO_TLS=1; shift ;;
    --source) FLEETDOCK_SOURCE="${2:?--source needs a value}"; shift 2 ;;
    --build) BUILD_LOCALLY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
done

# Validate before doing anything expensive — installing Docker takes minutes and
# should not happen only to fail on a bad flag combination.
if [ -n "$BUILD_LOCALLY" ]; then
  [ -n "$FLEETDOCK_SOURCE" ] || die "--build requires --source pointing at a Fleetdock checkout"
  [ -d "$FLEETDOCK_SOURCE" ] || die "--source '$FLEETDOCK_SOURCE' is not a directory; --build needs a local checkout"
  [ -f "${FLEETDOCK_SOURCE}/Dockerfile" ] || die "no Dockerfile in '$FLEETDOCK_SOURCE' — is it a Fleetdock checkout?"
fi

# --- platform checks ----------------------------------------------------------

OS="$(uname -s)"
case "$OS" in
  Linux) ;;
  Darwin)
    # macOS is supported for local evaluation only. A laptop has no public
    # address, so there is nothing to point DNS at and nothing for Let's Encrypt
    # to reach; and Docker Desktop NATs inbound connections, which breaks the
    # gateway's source-IP allowlists. Everything below adapts to that rather
    # than pretending it is a server install.
    MACOS=1
    NO_TLS=1
    [ -n "${FLEETDOCK_DIR_SET:-}" ] || FLEETDOCK_DIR="${HOME}/.fleetdock"
    ;;
  *) die "unsupported platform '$OS'. Fleetdock installs on Linux, and on macOS for local evaluation." ;;
esac

case "$(uname -m)" in
  x86_64|amd64|aarch64|arm64) ;;
  *) die "unsupported architecture $(uname -m)" ;;
esac

# Docker Desktop binds privileged ports as the invoking user and everything else
# lives under $HOME, so the macOS path needs no root at all.
if [ -z "${MACOS:-}" ] && [ "$(id -u)" != "0" ]; then
  # Re-exec only works when the script is on disk. Under `curl … | sh` the
  # script arrives on stdin and $0 is the shell itself, so there is nothing to
  # re-run — say what to type instead of failing obscurely.
  if command -v sudo >/dev/null 2>&1 && [ -f "$0" ] && [ -r "$0" ]; then
    step "Re-running with sudo"
    # shellcheck disable=SC2086
    exec sudo -E sh "$0" \
      ${FLEETDOCK_DOMAIN:+--domain "$FLEETDOCK_DOMAIN"} \
      --admin-email "$FLEETDOCK_ADMIN_EMAIL" \
      --dir "$FLEETDOCK_DIR" \
      --tag "$FLEETDOCK_RELEASE_TAG" \
      ${FLEETDOCK_SOURCE:+--source "$FLEETDOCK_SOURCE"} \
      ${WITH_GATEWAY:+--with-gateway} \
      ${NO_TLS:+--no-tls} \
      ${BUILD_LOCALLY:+--build}
  fi
  cat >&2 <<'EOF'
error: this installer must run as root.

  curl -sSL https://fleetdock.dev/install.sh | sudo sh

Put sudo after the pipe, not before curl.
EOF
  exit 1
fi

download() {
  # $1 = url, $2 = destination
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    die "need curl or wget"
  fi
}

fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsS --max-time 10 "$1" 2>/dev/null
  else
    wget -qO- --timeout=10 "$1" 2>/dev/null
  fi
}

# --- docker -------------------------------------------------------------------

step "Checking Docker"
if ! command -v docker >/dev/null 2>&1; then
  if [ -n "${MACOS:-}" ]; then
    # get.docker.com is Linux-only, and Docker Desktop is a GUI app that needs a
    # manual first run, so there is nothing sensible to automate here.
    die "Docker Desktop is required. Install it, start it, then re-run:

  brew install --cask docker

or download it from https://docker.com/products/docker-desktop"
  fi
  step "Installing Docker"
  download https://get.docker.com /tmp/get-docker.sh
  sh /tmp/get-docker.sh >/dev/null 2>&1 || die "Docker installation failed; install it manually and re-run"
  rm -f /tmp/get-docker.sh
fi
if [ -z "${MACOS:-}" ] && command -v systemctl >/dev/null 2>&1; then
  systemctl enable --now docker >/dev/null 2>&1 || true
fi
docker info >/dev/null 2>&1 || die "Docker is installed but not running${MACOS:+ — start Docker Desktop and re-run}"

# The compose file uses inline `configs: content:` (Compose 2.23) and
# `depends_on.required` (2.20). Older versions fail with an opaque YAML error,
# so check up front and say what to do about it.
compose_version="$(docker compose version --short 2>/dev/null || echo 0)"
compose_major="$(echo "$compose_version" | cut -d. -f1)"
compose_minor="$(echo "$compose_version" | cut -d. -f2)"
if [ "${compose_major:-0}" -lt 2 ] || { [ "${compose_major:-0}" -eq 2 ] && [ "${compose_minor:-0}" -lt 23 ]; }; then
  die "Docker Compose >= 2.23 required, found ${compose_version}. Upgrade the compose plugin: https://docs.docker.com/compose/install/"
fi

# --- existing install ---------------------------------------------------------

UPGRADE=""
if [ -f "${FLEETDOCK_DIR}/.env" ]; then
  UPGRADE=1
  step "Existing installation found in ${FLEETDOCK_DIR} — upgrading, keeping configuration"
fi

# --- port pre-flight ----------------------------------------------------------

if [ -z "$UPGRADE" ]; then
  port_in_use() {
    if command -v ss >/dev/null 2>&1; then
      ss -ltnH "sport = :$1" 2>/dev/null | grep -q . && return 0
    elif command -v lsof >/dev/null 2>&1; then
      # macOS: BSD netstat cannot filter by port, lsof can.
      lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1 && return 0
    elif command -v netstat >/dev/null 2>&1; then
      netstat -ltn 2>/dev/null | grep -qE "[:.]$1[[:space:]]" && return 0
    fi
    return 1
  }
  for p in 80 443; do
    if port_in_use "$p"; then
      die "port $p is already in use. Stop whatever is bound to it (often nginx or apache) and re-run."
    fi
  done
fi

# --- domain -------------------------------------------------------------------

detect_ip() {
  ip="$(fetch https://api.ipify.org || true)"
  [ -n "${ip:-}" ] || ip="$(fetch https://ifconfig.me/ip || true)"
  [ -n "${ip:-}" ] || ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  # macOS has no `hostname -I`; fall back to the primary interface address.
  [ -n "${ip:-}" ] || ip="$(ipconfig getifaddr en0 2>/dev/null || true)"
  echo "${ip:-}"
}

if [ -z "$UPGRADE" ]; then
  PUBLIC_IP="$(detect_ip)"

  if [ -n "$FLEETDOCK_DOMAIN" ]; then
    :
  elif [ -n "${MACOS:-}" ]; then
    # A laptop is not reachable from the internet, so there is no name worth
    # deriving. localhost is honest about what this install can serve. To let
    # agents on the same LAN reach it, pass the Mac's LAN address as --domain.
    FLEETDOCK_DOMAIN="localhost"
  elif [ -n "$NO_TLS" ] || [ -z "$PUBLIC_IP" ]; then
    [ -n "$PUBLIC_IP" ] || die "could not determine this host's IP address; pass --domain"
    FLEETDOCK_DOMAIN="$PUBLIC_IP"
    NO_TLS=1
  else
    # sslip.io resolves <a-b-c-d>.sslip.io to a.b.c.d, so Let's Encrypt can
    # issue a real certificate with no DNS setup at all. It is on the Public
    # Suffix List, so each name has its own rate-limit budget.
    FLEETDOCK_DOMAIN="$(echo "$PUBLIC_IP" | tr '.' '-').sslip.io"
    step "No --domain given; using ${FLEETDOCK_DOMAIN} (resolves to ${PUBLIC_IP})"
  fi

  if [ -n "$NO_TLS" ]; then
    SITE_ADDRESS="http://${FLEETDOCK_DOMAIN}"
    PUBLIC_URL="http://${FLEETDOCK_DOMAIN}"
    if [ -z "${MACOS:-}" ]; then
      warn "serving plain HTTP. Session tokens will cross the network in the clear — use --domain with a real hostname for TLS."
    fi
  else
    SITE_ADDRESS="$FLEETDOCK_DOMAIN"
    PUBLIC_URL="https://${FLEETDOCK_DOMAIN}"

    # A domain that does not point here yet is the most common reason ACME
    # fails. Warn, but do not block: DNS may still be propagating.
    if [ -n "$PUBLIC_IP" ] && command -v getent >/dev/null 2>&1; then
      resolved="$(getent hosts "$FLEETDOCK_DOMAIN" 2>/dev/null | awk '{print $1}' | head -1)"
      if [ -n "$resolved" ] && [ "$resolved" != "$PUBLIC_IP" ]; then
        warn "${FLEETDOCK_DOMAIN} resolves to ${resolved}, but this host appears to be ${PUBLIC_IP}. Certificate issuance will fail until DNS points here."
      elif [ -z "$resolved" ]; then
        warn "${FLEETDOCK_DOMAIN} does not resolve yet. Certificate issuance will fail until it does."
      fi
    fi
  fi
fi

# --- secrets ------------------------------------------------------------------

rand_b64() {
  # $1 = bytes of entropy
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 "$1" | tr -d '\n'
  elif command -v base64 >/dev/null 2>&1; then
    head -c "$1" /dev/urandom | base64 | tr -d '\n'
  else
    # Same entropy, longer string. No external tooling required.
    od -An -tx1 -N "$1" /dev/urandom | tr -d ' \n'
  fi
}

# --- write configuration ------------------------------------------------------

mkdir -p "$FLEETDOCK_DIR"
cd "$FLEETDOCK_DIR"

if [ "$FLEETDOCK_RELEASE_TAG" != "latest" ]; then
  COMPOSE_REF="$FLEETDOCK_RELEASE_TAG"
fi
[ -n "$FLEETDOCK_SOURCE" ] || \
  FLEETDOCK_SOURCE="https://raw.githubusercontent.com/${FLEETDOCK_REPO}/${COMPOSE_REF}"

# get_asset copies a repo-relative file from FLEETDOCK_SOURCE, which is either a
# local checkout or a base URL.
get_asset() {
  # $1 = path within the repo, $2 = destination
  if [ -d "$FLEETDOCK_SOURCE" ]; then
    [ -f "${FLEETDOCK_SOURCE}/$1" ] || die "${FLEETDOCK_SOURCE}/$1 not found — is --source a Fleetdock checkout?"
    cp "${FLEETDOCK_SOURCE}/$1" "$2"
  else
    download "${FLEETDOCK_SOURCE}/$1" "$2" || die "could not fetch ${FLEETDOCK_SOURCE}/$1

If the repository is private or the release is not published yet, install from a
local checkout instead:

  sudo sh install.sh --source /path/to/fleetdock --build"
  fi
}

step "Fetching stack definition"
get_asset docker-compose.yml "${FLEETDOCK_DIR}/docker-compose.yml"

if [ -z "$UPGRADE" ]; then
  step "Generating secrets"
  JWT_SECRET="$(rand_b64 48)"
  ENCRYPTION_KEY="$(rand_b64 32)"
  ADMIN_PASSWORD="$(rand_b64 16 | tr -d '/+=' | cut -c1-20)"

  if [ -n "$WITH_GATEWAY" ]; then
    GATEWAY_ENABLED=true
  else
    GATEWAY_ENABLED=false
  fi

  umask 077
  cat > "${FLEETDOCK_DIR}/.env" <<EOF
# Written by install.sh. Keep this file: it holds the only copy of the keys that
# decrypt stored database credentials.
#
# FLEETDOCK_ENCRYPTION_KEY in particular must never be regenerated — every
# credential and object-store key encrypted under it becomes unreadable.

FLEETDOCK_DOMAIN=${FLEETDOCK_DOMAIN}
FLEETDOCK_SITE_ADDRESS=${SITE_ADDRESS}
FLEETDOCK_PUBLIC_URL=${PUBLIC_URL}
FLEETDOCK_CORS_ORIGIN=${PUBLIC_URL}

FLEETDOCK_ENV=production
FLEETDOCK_RELEASE_TAG=${FLEETDOCK_RELEASE_TAG}

FLEETDOCK_JWT_SECRET=${JWT_SECRET}
FLEETDOCK_ENCRYPTION_KEY=${ENCRYPTION_KEY}
FLEETDOCK_ADMIN_EMAIL=${FLEETDOCK_ADMIN_EMAIL}
FLEETDOCK_ADMIN_PASSWORD=${ADMIN_PASSWORD}

# External database access. Enable with: fleetdock gateway enable
FLEETDOCK_GATEWAY_ENABLED=${GATEWAY_ENABLED}
EOF
  [ -n "$WITH_GATEWAY" ] && echo "COMPOSE_PROFILES=gateway" >> "${FLEETDOCK_DIR}/.env"
  chmod 600 "${FLEETDOCK_DIR}/.env"
else
  # Upgrade: read back what we need for the summary, change nothing.
  PUBLIC_URL="$(grep '^FLEETDOCK_PUBLIC_URL=' "${FLEETDOCK_DIR}/.env" | cut -d= -f2-)"
  FLEETDOCK_ADMIN_EMAIL="$(grep '^FLEETDOCK_ADMIN_EMAIL=' "${FLEETDOCK_DIR}/.env" | cut -d= -f2-)"
  ADMIN_PASSWORD=""
  # Keep the requested tag in sync without touching anything else.
  if grep -q '^FLEETDOCK_RELEASE_TAG=' "${FLEETDOCK_DIR}/.env"; then
    sed -i "s|^FLEETDOCK_RELEASE_TAG=.*|FLEETDOCK_RELEASE_TAG=${FLEETDOCK_RELEASE_TAG}|" "${FLEETDOCK_DIR}/.env"
  else
    echo "FLEETDOCK_RELEASE_TAG=${FLEETDOCK_RELEASE_TAG}" >> "${FLEETDOCK_DIR}/.env"
  fi
fi

# --- helper CLI ---------------------------------------------------------------

step "Installing the fleetdock command"
# /usr/local/bin needs root on macOS, and the macOS path deliberately does not
# ask for it. Fall back to a user bin and say so rather than failing.
CLI_DIR=/usr/local/bin
CLI_NOTE=""
if [ ! -w "$CLI_DIR" ]; then
  CLI_DIR="${HOME}/.local/bin"
  mkdir -p "$CLI_DIR"
  case ":${PATH}:" in
    *":${CLI_DIR}:"*) ;;
    *) CLI_NOTE="${CLI_DIR} is not on your PATH — add it, or call ${CLI_DIR}/fleetdock directly." ;;
  esac
fi
# Bake the real install directory into the copy, so `fleetdock` finds it after
# --dir or a macOS install under $HOME. Substituting on the way out avoids
# sed -i, which differs between GNU and BSD.
get_asset scripts/fleetdock "${FLEETDOCK_DIR}/.fleetdock-cli"
sed "s|^FLEETDOCK_DIR=.*|FLEETDOCK_DIR=\"\${FLEETDOCK_DIR:-${FLEETDOCK_DIR}}\"|" \
  "${FLEETDOCK_DIR}/.fleetdock-cli" > "${CLI_DIR}/fleetdock"
rm -f "${FLEETDOCK_DIR}/.fleetdock-cli"
chmod 755 "${CLI_DIR}/fleetdock"

# --- start --------------------------------------------------------------------

if [ -n "$BUILD_LOCALLY" ]; then
  # Image name must match what docker-compose.yml references, so compose finds
  # it locally and never reaches for the registry.
  step "Building the image from ${FLEETDOCK_SOURCE} (this takes a few minutes)"
  docker build -t "ghcr.io/fleetdock/fleetdock:${FLEETDOCK_RELEASE_TAG}" "$FLEETDOCK_SOURCE"
else
  step "Pulling images"
  docker compose --project-directory "$FLEETDOCK_DIR" --env-file "${FLEETDOCK_DIR}/.env" pull -q \
    || die "could not pull images. If no release has been published yet, build from a checkout instead:

  sudo sh install.sh --source /path/to/fleetdock --build"
fi

step "Starting Fleetdock"
docker compose --project-directory "$FLEETDOCK_DIR" --env-file "${FLEETDOCK_DIR}/.env" up -d

step "Waiting for the control plane"
ready=""
i=0
while [ $i -lt 90 ]; do
  if docker compose --project-directory "$FLEETDOCK_DIR" --env-file "${FLEETDOCK_DIR}/.env" \
      exec -T fleetdock wget -qO- http://127.0.0.1:8080/readyz >/dev/null 2>&1; then
    ready=1
    break
  fi
  i=$((i + 1))
  sleep 2
done
if [ -z "$ready" ]; then
  docker compose --project-directory "$FLEETDOCK_DIR" --env-file "${FLEETDOCK_DIR}/.env" logs --tail 50
  die "the control plane did not become ready. Logs above; configuration is in ${FLEETDOCK_DIR}/.env"
fi

# Certificate issuance happens on the first request, so this can lag readiness.
step "Waiting for the dashboard"
i=0
while [ $i -lt 60 ]; do
  if fetch "${PUBLIC_URL}/login" >/dev/null 2>&1; then break; fi
  i=$((i + 1))
  sleep 2
done
if ! fetch "${PUBLIC_URL}/login" >/dev/null 2>&1; then
  warn "the dashboard is not reachable at ${PUBLIC_URL} yet."
  warn "Check DNS and the host firewall, then: fleetdock logs caddy"
fi

# --- summary ------------------------------------------------------------------

cat <<EOF

  Fleetdock is running.

    Dashboard   ${PUBLIC_URL}
    Email       ${FLEETDOCK_ADMIN_EMAIL}
EOF
if [ -n "${ADMIN_PASSWORD:-}" ]; then
  cat <<EOF
    Password    ${ADMIN_PASSWORD}

  This password is shown once. It is also in ${FLEETDOCK_DIR}/.env.
EOF
else
  echo "    Password    unchanged"
fi
cat <<EOF

  Manage it with:  fleetdock status | logs | update | doctor
  Connect a server: Servers -> Connect server in the dashboard.
EOF
[ -n "${CLI_NOTE:-}" ] && printf '\n  %s\n' "$CLI_NOTE"
if [ -n "${MACOS:-}" ]; then
  cat <<EOF

  This is a local evaluation install. Three limits come from macOS, not from
  Fleetdock:

    - No TLS. Docker Desktop has no public address for Let's Encrypt to reach.
    - Servers elsewhere cannot enrol: they need to reach ${PUBLIC_URL}. For
      agents on your LAN, reinstall with --domain <your-mac's-LAN-IP>.
    - External database access does not work. Docker Desktop NATs inbound
      connections, so the gateway sees its VM's address instead of the client's
      and every CIDR allowlist rejects everyone.

  For a real deployment, run the same command on a Linux host.
EOF
fi
echo
