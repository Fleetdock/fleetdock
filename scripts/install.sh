#!/bin/sh
# Fleetdock installer.
#
#   curl -sSL https://fleetdock.dev/install.sh | sh
#   curl -sSL https://fleetdock.dev/install.sh | sh -s -- --domain db.example.com
#   curl -sSL https://fleetdock.dev/install.sh | sh -s -- --local
#
# Not to be confused with the *agent* installer, which your own Fleetdock
# serves at https://<your-fleetdock>/install.sh and which you run on each
# database server.
#
# Two kinds of install:
#
#   server  A Linux host other people and your database servers can reach.
#           Automatic HTTPS behind Caddy; ports 80 and 443 on every interface.
#   local   Only this computer (macOS, Linux, or Windows through WSL2). Plain
#           HTTP on http://localhost, reachable from this machine alone. Chosen
#           automatically on macOS and WSL, asked about elsewhere.
#
# Run in a terminal it asks a few questions first; --yes (or no terminal at
# all, as under CI or cloud-init) takes the defaults and asks nothing.
set -eu

FLEETDOCK_REPO="${FLEETDOCK_REPO:-fleetdock/fleetdock}"
# Where docker-compose.yml and the fleetdock CLI come from. Defaults to the
# published repo, but may point at a local checkout — which is what makes this
# installable from a fork, from an air-gapped mirror, or in a test VM before
# anything has been published.
FLEETDOCK_SOURCE="${FLEETDOCK_SOURCE:-}"
BUILD_LOCALLY=""
FLEETDOCK_DIR="${FLEETDOCK_DIR:-/opt/fleetdock}"
# TAG_EXPLICIT records whether a tag was asked for (--tag or the environment):
# re-running the installer to upgrade must not silently move a pinned install
# back to "latest".
TAG_EXPLICIT="${FLEETDOCK_RELEASE_TAG:+1}"
FLEETDOCK_RELEASE_TAG="${FLEETDOCK_RELEASE_TAG:-latest}"
FLEETDOCK_DOMAIN="${FLEETDOCK_DOMAIN:-}"
EMAIL_SET="${FLEETDOCK_ADMIN_EMAIL:+1}"
FLEETDOCK_ADMIN_EMAIL="${FLEETDOCK_ADMIN_EMAIL:-admin@example.com}"
WITH_GATEWAY=""
NO_TLS=""
COMPOSE_REF="main"
# MODE is "server", "local" or empty (not decided yet — ask, or default).
MODE=""
case "${FLEETDOCK_MODE:-}" in server|local) MODE="$FLEETDOCK_MODE" ;; esac
LOCAL_PORT="${FLEETDOCK_PORT:-}"
ASSUME_YES="${FLEETDOCK_NONINTERACTIVE:-}"

# --- output helpers -----------------------------------------------------------

step() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

# --- argument parsing ---------------------------------------------------------

usage() {
  cat <<'EOF'
Usage: install.sh [options]

Where it runs:
  --local             Only this computer: http://localhost, reachable from this
                      machine alone. Default on macOS and Windows (WSL2).
  --server            A server others reach over HTTPS. Default on Linux when
                      there is no terminal to ask.
  --port <n>          Local installs: serve on http://localhost:<n> instead of
                      port 80 (for when 80 is already taken).

Server installs:
  --domain <host>     Hostname for the dashboard. Gets an automatic TLS
                      certificate. Without it, an <ip>.sslip.io name is used so
                      HTTPS still works with no DNS setup.
  --no-tls            Serve plain HTTP on the host's IP. No certificate.
  --with-gateway      Also start the external database access gateway. Adds a
                      50-port range; off by default.

Everything:
  --admin-email <e>   Bootstrap admin account (default admin@example.com).
  --dir <path>        Install directory (default /opt/fleetdock, or
                      ~/.fleetdock for a local install without root).
  --tag <tag>         Image tag to run (default latest).
  -y, --yes           Ask nothing; take the defaults and the options given.
  --source <path|url> Take docker-compose.yml and the fleetdock CLI from here
                      instead of the published repo. A local checkout works.
  --build             Build the application image from --source instead of
                      pulling it. Requires --source to be a checkout.
  -h, --help          This message.

Every option also reads from the matching FLEETDOCK_* environment variable, so
`curl -sSL … | FLEETDOCK_DOMAIN=db.example.com sh` works too
(FLEETDOCK_NONINTERACTIVE=1 is the same as --yes).
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --domain) FLEETDOCK_DOMAIN="${2:?--domain needs a value}"; shift 2 ;;
    --admin-email) FLEETDOCK_ADMIN_EMAIL="${2:?--admin-email needs a value}"; EMAIL_SET=1; shift 2 ;;
    --dir) FLEETDOCK_DIR="${2:?--dir needs a value}"; FLEETDOCK_DIR_SET=1; shift 2 ;;
    --tag) FLEETDOCK_RELEASE_TAG="${2:?--tag needs a value}"; TAG_EXPLICIT=1; shift 2 ;;
    --with-gateway) WITH_GATEWAY=1; shift ;;
    --no-tls) NO_TLS=1; shift ;;
    --local) MODE=local; shift ;;
    --server) MODE=server; shift ;;
    --port) LOCAL_PORT="${2:?--port needs a value}"; shift 2 ;;
    -y|--yes) ASSUME_YES=1; shift ;;
    --source) FLEETDOCK_SOURCE="${2:?--source needs a value}"; shift 2 ;;
    --build) BUILD_LOCALLY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
done
[ -n "${FLEETDOCK_DIR_SET:-}" ] || [ "${FLEETDOCK_DIR}" = /opt/fleetdock ] || FLEETDOCK_DIR_SET=1

# Validate before doing anything expensive — installing Docker takes minutes and
# should not happen only to fail on a bad flag combination.
if [ -n "$BUILD_LOCALLY" ]; then
  [ -n "$FLEETDOCK_SOURCE" ] || die "--build requires --source pointing at a Fleetdock checkout"
  [ -d "$FLEETDOCK_SOURCE" ] || die "--source '$FLEETDOCK_SOURCE' is not a directory; --build needs a local checkout"
  [ -f "${FLEETDOCK_SOURCE}/Dockerfile" ] || die "no Dockerfile in '$FLEETDOCK_SOURCE' — is it a Fleetdock checkout?"
fi
if [ -n "$LOCAL_PORT" ]; then
  case "$LOCAL_PORT" in
    *[!0-9]*|"") die "--port needs a number, got '$LOCAL_PORT'" ;;
  esac
  [ "$LOCAL_PORT" -ge 1 ] && [ "$LOCAL_PORT" -le 65535 ] || die "--port must be between 1 and 65535"
  [ "$MODE" != server ] || die "--port is for local installs; a server install serves on 80 and 443"
fi

# --- platform checks ----------------------------------------------------------

# interactive is true when a person can answer questions. Under `curl … | sh`
# stdin is the script itself, so questions are asked on /dev/tty — and when
# there is none (CI, cloud-init, ssh without -t) nothing is asked at all.
interactive() {
  [ -z "$ASSUME_YES" ] && [ -r /dev/tty ] && ( : >/dev/tty ) 2>/dev/null
}

# ask prints a question on the terminal and reads the answer into $REPLY.
ask() {
  printf '%s' "$1" >/dev/tty
  REPLY=""
  read -r REPLY </dev/tty || REPLY=""
}

OS="$(uname -s)"
case "$OS" in
  Linux)
    # WSL2 is Windows underneath: whatever runs here is a developer machine,
    # not a server anyone else reaches, and Docker usually comes from Docker
    # Desktop on the Windows side.
    if grep -qi microsoft /proc/version 2>/dev/null; then
      WSL=1
      [ -n "$MODE" ] || MODE=local
    fi
    ;;
  Darwin)
    # A laptop has no public address, so there is nothing to point DNS at and
    # nothing for Let's Encrypt to reach; and Docker Desktop NATs inbound
    # connections, which breaks the gateway's source-IP allowlists.
    MACOS=1
    [ "$MODE" != server ] || die "macOS installs are local only — a Mac cannot serve HTTPS to other hosts. Use a Linux server, or drop --server."
    MODE=local
    ;;
  MINGW*|MSYS*|CYGWIN*)
    die "Windows is supported through WSL2, not Git Bash or MSYS.

  1. Install Docker Desktop and, in Settings -> Resources -> WSL integration,
     turn it on for your Linux distribution.
  2. Open the Ubuntu (WSL) terminal and run:

       curl -sSL https://fleetdock.dev/install.sh | sh

Fleetdock is then at http://localhost in your Windows browser."
    ;;
  *) die "unsupported platform '$OS'. Fleetdock installs on Linux, macOS (local), and Windows through WSL2." ;;
esac

case "$(uname -m)" in
  x86_64|amd64|aarch64|arm64) ;;
  *) die "unsupported architecture $(uname -m)" ;;
esac

docker_usable() { command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; }

# existing_env prints the .env of a previous install, if there is one, so
# re-running the installer to upgrade keeps the kind of install it already is
# instead of asking again.
existing_env() {
  if [ -f "${FLEETDOCK_DIR}/.env" ]; then
    echo "${FLEETDOCK_DIR}/.env"
  elif [ -z "${FLEETDOCK_DIR_SET:-}" ] && [ -f "${HOME}/.fleetdock/.env" ]; then
    echo "${HOME}/.fleetdock/.env"
  fi
}

# --- install kind -------------------------------------------------------------

PREVIOUS_ENV="$(existing_env 2>/dev/null || true)"
if [ -z "$MODE" ] && [ -n "$PREVIOUS_ENV" ]; then
  case "$(grep '^FLEETDOCK_MODE=' "$PREVIOUS_ENV" 2>/dev/null | cut -d= -f2-)" in
    local) MODE=local ;;
    server) MODE=server ;;
  esac
fi
if [ -z "$MODE" ] && [ -n "$FLEETDOCK_DOMAIN" ]; then
  MODE=server
fi
if [ -z "$MODE" ] && [ -z "$PREVIOUS_ENV" ] && interactive; then
  cat >/dev/tty <<'EOF'

Where are you installing Fleetdock?

  1) A server that other people and your database servers can reach
     (HTTPS, ports 80 and 443)
  2) Only this computer — private, at http://localhost

EOF
  while :; do
    ask "Choose 1 or 2 [1]: "
    case "$REPLY" in
      ""|1) MODE=server; break ;;
      2) MODE=local; break ;;
    esac
  done
fi
# Without a terminal the historic behaviour stands: a Linux host is a server.
[ -n "$MODE" ] || MODE=server
if [ "$MODE" = local ]; then
  LOCAL=1
  NO_TLS=1
fi

# --- privileges ---------------------------------------------------------------

# A local install needs no root when Docker already works for this user: it
# lives under $HOME and drives Docker as the invoking user. Docker Desktop
# (macOS, WSL) always works that way; a Linux machine does once the user is in
# the docker group. Everything else needs root, to install Docker and to write
# /opt/fleetdock and /usr/local/bin.
NEED_ROOT=1
if [ -n "${LOCAL:-}" ] && { [ -n "${MACOS:-}" ] || [ -n "${WSL:-}" ] || docker_usable; }; then
  NEED_ROOT=""
fi

if [ -n "$NEED_ROOT" ] && [ "$(id -u)" != "0" ]; then
  # Re-exec only works when the script is on disk. Under `curl … | sh` the
  # script arrives on stdin and $0 is the shell itself, so there is nothing to
  # re-run — say what to type instead of failing obscurely.
  if command -v sudo >/dev/null 2>&1 && [ -f "$0" ] && [ -r "$0" ]; then
    step "Re-running with sudo"
    # shellcheck disable=SC2086
    exec sudo -E sh "$0" \
      ${FLEETDOCK_DOMAIN:+--domain "$FLEETDOCK_DOMAIN"} \
      --admin-email "$FLEETDOCK_ADMIN_EMAIL" \
      ${FLEETDOCK_DIR_SET:+--dir "$FLEETDOCK_DIR"} \
      --tag "$FLEETDOCK_RELEASE_TAG" \
      ${FLEETDOCK_SOURCE:+--source "$FLEETDOCK_SOURCE"} \
      "--${MODE}" \
      ${LOCAL_PORT:+--port "$LOCAL_PORT"} \
      ${WITH_GATEWAY:+--with-gateway} \
      ${NO_TLS:+--no-tls} \
      ${ASSUME_YES:+--yes} \
      ${BUILD_LOCALLY:+--build}
  fi
  if [ -n "${LOCAL:-}" ]; then
    cat >&2 <<'EOF'
error: Docker is not usable by this user, so the installer needs root to set it up.

  curl -sSL https://fleetdock.dev/install.sh | sudo sh -s -- --local

Or, if Docker is installed, add yourself to the docker group, log out and in
again, and re-run without sudo:

  sudo usermod -aG docker "$USER"
EOF
  else
    cat >&2 <<'EOF'
error: a server install must run as root.

  curl -sSL https://fleetdock.dev/install.sh | sudo sh

Put sudo after the pipe, not before curl. To install only for yourself on this
computer instead, add --local:

  curl -sSL https://fleetdock.dev/install.sh | sh -s -- --local
EOF
  fi
  exit 1
fi

# A local install by a non-root user lives in their home directory.
if [ -n "${LOCAL:-}" ] && [ -z "${FLEETDOCK_DIR_SET:-}" ] && [ "$(id -u)" != "0" ]; then
  FLEETDOCK_DIR="${HOME}/.fleetdock"
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

# --- existing install ---------------------------------------------------------

UPGRADE=""
if [ -f "${FLEETDOCK_DIR}/.env" ]; then
  UPGRADE=1
  step "Existing installation found in ${FLEETDOCK_DIR} — upgrading, keeping configuration"
fi

# --- questions ----------------------------------------------------------------

detect_ip() {
  ip="$(fetch https://api.ipify.org || true)"
  [ -n "${ip:-}" ] || ip="$(fetch https://ifconfig.me/ip || true)"
  [ -n "${ip:-}" ] || ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  # macOS has no `hostname -I`; fall back to the primary interface address.
  [ -n "${ip:-}" ] || ip="$(ipconfig getifaddr en0 2>/dev/null || true)"
  echo "${ip:-}"
}

PUBLIC_IP=""
if [ -z "$UPGRADE" ] && [ -z "${LOCAL:-}" ]; then
  PUBLIC_IP="$(detect_ip)"
fi

if [ -z "$UPGRADE" ] && interactive; then
  if [ -z "${LOCAL:-}" ] && [ -z "$FLEETDOCK_DOMAIN" ] && [ -z "$NO_TLS" ]; then
    if [ -n "$PUBLIC_IP" ]; then
      auto="$(echo "$PUBLIC_IP" | tr '.' '-').sslip.io"
      printf '\nThe dashboard needs a hostname. Point a DNS record at %s first, or press\nEnter to use %s (free HTTPS, no DNS setup).\n\n' "$PUBLIC_IP" "$auto" >/dev/tty
      ask "Domain [${auto}]: "
    else
      printf '\nThe dashboard needs a hostname pointing at this server.\n\n' >/dev/tty
      ask "Domain: "
    fi
    REPLY="${REPLY#https://}"; REPLY="${REPLY#http://}"; REPLY="${REPLY%%/*}"
    [ -z "$REPLY" ] || FLEETDOCK_DOMAIN="$REPLY"
  fi
  if [ -z "$EMAIL_SET" ]; then
    ask "Admin email [${FLEETDOCK_ADMIN_EMAIL}]: "
    [ -z "$REPLY" ] || FLEETDOCK_ADMIN_EMAIL="$REPLY"
  fi
fi

# --- port pre-flight ----------------------------------------------------------

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

if [ -z "$UPGRADE" ]; then
  if [ -n "${LOCAL:-}" ]; then
    # A local install publishes one port. HTTPS gets a random loopback port, as
    # nothing uses it, so 443 being taken does not matter.
    p="${LOCAL_PORT:-80}"
    if port_in_use "$p"; then
      die "port $p is already in use on this computer. Pick another, for example:

  curl -sSL https://fleetdock.dev/install.sh | sh -s -- --local --port 8090"
    fi
  else
    for p in 80 443; do
      if port_in_use "$p"; then
        die "port $p is already in use. Stop whatever is bound to it (often nginx or apache) and re-run."
      fi
    done
  fi
fi

# --- domain -------------------------------------------------------------------

HTTP_BIND=""
HTTPS_BIND=""
if [ -z "$UPGRADE" ]; then
  if [ -n "${LOCAL:-}" ]; then
    # Nothing on the internet can reach this machine, so there is no name worth
    # deriving and no certificate to get. Bind to loopback, so the plain-HTTP
    # dashboard is not exposed to everyone on the same network. A --domain
    # here (a LAN or VPN address, so agents nearby can enrol) is the one
    # reason to listen on every interface.
    port="${LOCAL_PORT:-80}"
    if [ -n "$FLEETDOCK_DOMAIN" ]; then
      HTTP_BIND="$port"
    else
      FLEETDOCK_DOMAIN="localhost"
      HTTP_BIND="127.0.0.1:${port}"
    fi
    # Empty host port: Docker picks a free loopback port for the unused 443.
    HTTPS_BIND="127.0.0.1:"
    SITE_ADDRESS="http://${FLEETDOCK_DOMAIN}"
    PUBLIC_URL="http://${FLEETDOCK_DOMAIN}"
    [ "$port" = 80 ] || PUBLIC_URL="${PUBLIC_URL}:${port}"
  else
    if [ -n "$FLEETDOCK_DOMAIN" ]; then
      :
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
      warn "serving plain HTTP. Session tokens will cross the network in the clear — use --domain with a real hostname for TLS."
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
fi

# --- confirm ------------------------------------------------------------------

if [ -z "$UPGRADE" ] && interactive; then
  if [ -n "${LOCAL:-}" ]; then
    kind="Only this computer"
    if [ "${HTTP_BIND#127.0.0.1:}" != "$HTTP_BIND" ]; then
      ports="${HTTP_BIND#127.0.0.1:} (this computer only)"
    else
      ports="${HTTP_BIND} (every network interface)"
    fi
  else
    kind="Server"
    ports="80 and 443 (every network interface)"
  fi
  if docker_usable; then
    docker_note="already installed"
  elif command -v docker >/dev/null 2>&1; then
    docker_note="installed, not running"
  else
    docker_note="will be installed"
  fi
  cat >/dev/tty <<EOF

Fleetdock will be installed like this:

  Kind        ${kind}
  Dashboard   ${PUBLIC_URL}
  Admin       ${FLEETDOCK_ADMIN_EMAIL}
  Directory   ${FLEETDOCK_DIR}
  Ports       ${ports}
  Docker      ${docker_note}

EOF
  ask "Continue? [Y/n] "
  case "$REPLY" in
    ""|y|Y|yes|YES|Yes) ;;
    *) echo "Aborted; nothing was changed." >&2; exit 1 ;;
  esac
fi

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
  if [ -n "${WSL:-}" ]; then
    die "Docker is not available in this WSL distribution. Install Docker Desktop
for Windows, start it, and in Settings -> Resources -> WSL integration turn it
on for this distribution. Then re-run this command.

  https://docker.com/products/docker-desktop"
  fi
  [ "$(id -u)" = "0" ] || die "Docker is not installed. Re-run with sudo to install it, or install it yourself: https://docs.docker.com/engine/install/"
  step "Installing Docker"
  download https://get.docker.com /tmp/get-docker.sh
  sh /tmp/get-docker.sh >/dev/null 2>&1 || die "Docker installation failed; install it manually and re-run"
  rm -f /tmp/get-docker.sh
fi
if [ -z "${MACOS:-}" ] && [ -z "${WSL:-}" ] && [ "$(id -u)" = "0" ] && command -v systemctl >/dev/null 2>&1; then
  systemctl enable --now docker >/dev/null 2>&1 || true
fi
if ! docker info >/dev/null 2>&1; then
  if [ -n "${MACOS:-}" ] || [ -n "${WSL:-}" ]; then
    die "Docker is installed but not running — start Docker Desktop and re-run"
  fi
  die "Docker is installed but not running"
fi

# The compose file uses inline `configs: content:` (Compose 2.23) and
# `depends_on.required` (2.20). Older versions fail with an opaque YAML error,
# so check up front and say what to do about it.
compose_version="$(docker compose version --short 2>/dev/null || echo 0)"
compose_major="$(echo "$compose_version" | cut -d. -f1)"
compose_minor="$(echo "$compose_version" | cut -d. -f2)"
if [ "${compose_major:-0}" -lt 2 ] || { [ "${compose_major:-0}" -eq 2 ] && [ "${compose_minor:-0}" -lt 23 ]; }; then
  die "Docker Compose >= 2.23 required, found ${compose_version}. Upgrade the compose plugin: https://docs.docker.com/compose/install/"
fi

# --- secrets ------------------------------------------------------------------

rand_hex() {
  # $1 = bytes of entropy; hex output needs no escaping anywhere.
  od -An -tx1 -N "$1" /dev/urandom | tr -d ' \n'
}

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
  # Hex, so it needs no escaping inside the database URL.
  POSTGRES_PASSWORD="$(rand_hex 24)"

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

FLEETDOCK_MODE=${MODE}
FLEETDOCK_DOMAIN=${FLEETDOCK_DOMAIN}
FLEETDOCK_SITE_ADDRESS=${SITE_ADDRESS}
FLEETDOCK_PUBLIC_URL=${PUBLIC_URL}
FLEETDOCK_CORS_ORIGIN=${PUBLIC_URL}

FLEETDOCK_ENV=production
FLEETDOCK_RELEASE_TAG=${FLEETDOCK_RELEASE_TAG}

FLEETDOCK_JWT_SECRET=${JWT_SECRET}
FLEETDOCK_ENCRYPTION_KEY=${ENCRYPTION_KEY}

# Password of the bundled metadata Postgres (never published outside Docker).
FLEETDOCK_POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
FLEETDOCK_ADMIN_EMAIL=${FLEETDOCK_ADMIN_EMAIL}
FLEETDOCK_ADMIN_PASSWORD=${ADMIN_PASSWORD}

# External database access. Enable with: fleetdock gateway enable
FLEETDOCK_GATEWAY_ENABLED=${GATEWAY_ENABLED}
EOF
  if [ -n "$HTTP_BIND" ]; then
    cat >> "${FLEETDOCK_DIR}/.env" <<EOF

# Where the dashboard listens on this computer. 127.0.0.1 keeps it private to
# this machine; drop the address to listen on every interface.
FLEETDOCK_HTTP_BIND=${HTTP_BIND}
FLEETDOCK_HTTPS_BIND=${HTTPS_BIND}
EOF
  fi
  [ -n "$WITH_GATEWAY" ] && echo "COMPOSE_PROFILES=gateway" >> "${FLEETDOCK_DIR}/.env"
  chmod 600 "${FLEETDOCK_DIR}/.env"
else
  # Upgrade: read back what we need for the summary, change nothing.
  PUBLIC_URL="$(grep '^FLEETDOCK_PUBLIC_URL=' "${FLEETDOCK_DIR}/.env" | cut -d= -f2-)"
  FLEETDOCK_ADMIN_EMAIL="$(grep '^FLEETDOCK_ADMIN_EMAIL=' "${FLEETDOCK_DIR}/.env" | cut -d= -f2-)"
  case "$(grep '^FLEETDOCK_MODE=' "${FLEETDOCK_DIR}/.env" | cut -d= -f2-)" in
    local) LOCAL=1 ;;
    server) LOCAL="" ;;
  esac
  ADMIN_PASSWORD=""
  # Change the tag only when one was asked for; otherwise keep what the
  # install is pinned to. Rewritten with awk, not `sed -i` (not portable to
  # BSD sed on macOS).
  if [ -n "$TAG_EXPLICIT" ]; then
    tmp="$(mktemp "${FLEETDOCK_DIR}/.env.XXXXXX")"
    FD_VAL="$FLEETDOCK_RELEASE_TAG" awk '
      BEGIN { v = ENVIRON["FD_VAL"]; done = 0 }
      index($0, "FLEETDOCK_RELEASE_TAG=") == 1 { print "FLEETDOCK_RELEASE_TAG=" v; done = 1; next }
      { print }
      END { if (!done) print "FLEETDOCK_RELEASE_TAG=" v }
    ' "${FLEETDOCK_DIR}/.env" > "$tmp"
    chmod 600 "$tmp"
    mv "$tmp" "${FLEETDOCK_DIR}/.env"
  else
    FLEETDOCK_RELEASE_TAG="$(grep '^FLEETDOCK_RELEASE_TAG=' "${FLEETDOCK_DIR}/.env" | cut -d= -f2-)"
    FLEETDOCK_RELEASE_TAG="${FLEETDOCK_RELEASE_TAG:-latest}"
  fi
fi

# --- helper CLI ---------------------------------------------------------------

step "Installing the fleetdock command"
# /usr/local/bin needs root, and a local install deliberately does not ask for
# it. Fall back to a user bin and say so rather than failing.
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
# --dir or a local install under $HOME. Substituting on the way out avoids
# sed -i, which differs between GNU and BSD.
get_asset scripts/fleetdock "${FLEETDOCK_DIR}/.fleetdock-cli"
sed "s|^FLEETDOCK_DIR=.*|FLEETDOCK_DIR=\"\${FLEETDOCK_DIR:-${FLEETDOCK_DIR}}\"|" \
  "${FLEETDOCK_DIR}/.fleetdock-cli" > "${CLI_DIR}/fleetdock"
rm -f "${FLEETDOCK_DIR}/.fleetdock-cli"
chmod 755 "${CLI_DIR}/fleetdock"

# --- start --------------------------------------------------------------------

dc() {
  docker compose --project-directory "$FLEETDOCK_DIR" --env-file "${FLEETDOCK_DIR}/.env" "$@"
}

if [ -n "$BUILD_LOCALLY" ]; then
  # Image name must match what docker-compose.yml references, so compose finds
  # it locally and never reaches for the registry.
  step "Building the image from ${FLEETDOCK_SOURCE} (this takes a few minutes)"
  docker build -t "ghcr.io/fleetdock/fleetdock:${FLEETDOCK_RELEASE_TAG}" "$FLEETDOCK_SOURCE"
else
  step "Pulling images"
  dc pull -q \
    || die "could not pull images. If no release has been published yet, build from a checkout instead:

  sudo sh install.sh --source /path/to/fleetdock --build"
fi

# Postgres reads POSTGRES_PASSWORD only when it first creates its volume, so the
# password in .env and the one the database actually has can disagree — an
# install made before docker-compose.yml read FLEETDOCK_POSTGRES_PASSWORD has
# the generated value in .env but "fleetdock" in the database. Set the role's
# password to what .env says before the control plane connects with it. The
# container's local socket uses trust authentication, so this works whatever
# the old password was, and repeating it changes nothing.
if [ -n "$UPGRADE" ]; then
  pg_pw="$(grep '^FLEETDOCK_POSTGRES_PASSWORD=' "${FLEETDOCK_DIR}/.env" | cut -d= -f2-)"
  db_url="$(grep '^FLEETDOCK_DATABASE_URL=' "${FLEETDOCK_DIR}/.env" | cut -d= -f2-)"
  case "$db_url" in
    ""|*@postgres:*)
      if [ -n "$pg_pw" ]; then
        step "Checking the metadata database password"
        dc up -d postgres >/dev/null 2>&1 || true
        i=0
        until dc exec -T postgres pg_isready -U fleetdock -d fleetdock >/dev/null 2>&1; do
          i=$((i + 1)); [ $i -lt 30 ] || break; sleep 2
        done
        echo "ALTER ROLE fleetdock PASSWORD :'pw';" \
          | dc exec -T postgres psql -U fleetdock -d fleetdock -q -v ON_ERROR_STOP=1 -v pw="$pg_pw" >/dev/null \
          || warn "could not set the metadata database password; if the control plane cannot log in, run: fleetdock start"
      fi
      ;;
  esac
fi

step "Starting Fleetdock"
dc up -d

step "Waiting for the control plane"
ready=""
i=0
while [ $i -lt 90 ]; do
  if dc exec -T fleetdock wget -qO- http://127.0.0.1:8080/readyz >/dev/null 2>&1; then
    ready=1
    break
  fi
  i=$((i + 1))
  sleep 2
done
if [ -z "$ready" ]; then
  dc logs --tail 50
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
# The bootstrap admin is only created while the users table is empty, so against
# a pre-existing database the generated password is written to .env and never
# applied. Printing it regardless would hand over a credential that cannot log
# in. The API logs the account it creates, so use that as the signal.
BOOTSTRAPPED=""
if dc logs fleetdock 2>/dev/null | grep -q "bootstrapped admin account"; then
  BOOTSTRAPPED=1
fi

if [ -n "${ADMIN_PASSWORD:-}" ] && [ -n "$BOOTSTRAPPED" ]; then
  cat <<EOF
    Password    ${ADMIN_PASSWORD}
EOF
elif [ -n "${ADMIN_PASSWORD:-}" ]; then
  cat <<EOF

  The database already had an account, so no admin was created and the generated
  password was not applied. Sign in with your existing credentials.
EOF
else
  cat <<EOF
    Password    unchanged by this upgrade
EOF
fi
cat <<EOF

  Retrieve these any time with:  fleetdock credentials
  They are stored in ${FLEETDOCK_DIR}/.env — back it up (fleetdock backup-config).
EOF

if [ -z "$UPGRADE" ]; then
  if [ -n "${LOCAL:-}" ]; then
    cat <<EOF

  Next steps

    1. Open ${PUBLIC_URL} and sign in.
    2. Change the password: your name (top right) -> Your profile.
    3. Databases -> Connect database server -> Connect to one anywhere.
       For a database on this same computer, use host.docker.internal as the
       host — "localhost" would mean the Fleetdock container itself.
EOF
  else
    cat <<EOF

  Next steps

    1. Open ${PUBLIC_URL} and sign in.
    2. Change the password: your name (top right) -> Your profile.
    3. Servers -> Connect server, and run the command it shows on each database
       server. Or Databases -> Connect database server for one you reach
       directly.
EOF
  fi
fi
cat <<EOF

  Manage it with:  fleetdock status | logs | update | doctor
EOF
[ -n "${CLI_NOTE:-}" ] && printf '\n  %s\n' "$CLI_NOTE"
if [ -n "${LOCAL:-}" ] && [ -z "$UPGRADE" ]; then
  cat <<EOF

  This install is for this computer only. That means:

    - Your data stays here, in Docker volumes, and stored credentials are
      encrypted with the key in ${FLEETDOCK_DIR}/.env.
    - Servers elsewhere cannot connect their agents to it. Use it with
      databases this computer can reach (here, on your network, or through a
      VPN), or install on a server for the full fleet.
    - Scheduled backups and alerts run only while this computer is on and
      Docker is running.
EOF
  if [ -n "${MACOS:-}" ] || [ -n "${WSL:-}" ]; then
    cat <<EOF
    - External database access (the gateway) does not work on Docker Desktop.
EOF
  fi
fi
echo
