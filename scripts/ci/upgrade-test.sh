#!/bin/sh
# Upgrade test: run the previous release's API on an empty metadata database,
# create data through its API, stop it, start this checkout's API on the same
# database and check that everything is still there and still works.
#
# Usage: scripts/ci/upgrade-test.sh [OLD_REF]
#   OLD_REF      tag to upgrade from (default: the newest release tag before this commit)
#   DATABASE_URL an empty PostgreSQL database (required)
# Needs: git (with tags), go, curl, jq.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# The newest release that is not this commit (on a release tag, the one before).
OLD_REF="${1:-$(git -C "$ROOT" tag --list 'v*' --sort=-v:refname --no-contains HEAD | grep -v -- - | head -n 1)}"
: "${DATABASE_URL:?set DATABASE_URL to an empty PostgreSQL database}"
[ -n "$OLD_REF" ] || { echo "no release tag to upgrade from" >&2; exit 1; }

WORK="$(mktemp -d)"
ADDR=127.0.0.1:18080
BASE="http://$ADDR"
PID=""
cleanup() {
  [ -n "$PID" ] && kill "$PID" 2>/dev/null || true
  git -C "$ROOT" worktree remove --force "$WORK/old" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

export FLEETDOCK_DATABASE_URL="$DATABASE_URL"
export FLEETDOCK_HTTP_ADDR="$ADDR"
export FLEETDOCK_JWT_SECRET="upgrade-test-jwt-secret-0123456789abcdef"
export FLEETDOCK_ENCRYPTION_KEY="upgrade-test-encryption-key-0123456789abcdef"
export FLEETDOCK_ADMIN_EMAIL="admin@upgrade.test"
export FLEETDOCK_ADMIN_PASSWORD="upgrade-test-password"
export FLEETDOCK_WORKER_ENABLED=false

echo "==> Building $OLD_REF and this checkout"
git -C "$ROOT" worktree add --detach "$WORK/old" "$OLD_REF" >/dev/null
(cd "$WORK/old/backend" && go build -o "$WORK/api-old" ./cmd/api)
(cd "$ROOT/backend" && go build -o "$WORK/api-new" ./cmd/api)

start() {
  "$1" >"$WORK/$2.log" 2>&1 &
  PID=$!
  i=0
  until curl -fsS "$BASE/readyz" >/dev/null 2>&1; do
    i=$((i + 1))
    if [ "$i" -gt 60 ] || ! kill -0 "$PID" 2>/dev/null; then
      echo "$2 did not become ready:" >&2
      cat "$WORK/$2.log" >&2
      exit 1
    fi
    sleep 1
  done
}
stop() {
  kill "$PID"
  wait "$PID" 2>/dev/null || true
  PID=""
}
api() { # METHOD PATH [JSON] — authenticated with $TOKEN
  if [ $# -ge 3 ]; then
    curl -fsS -X "$1" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "$3" "$BASE$2"
  else
    curl -fsS -X "$1" -H "Authorization: Bearer $TOKEN" "$BASE$2"
  fi
}
login() {
  curl -fsS -H 'Content-Type: application/json' \
    -d "{\"email\":\"$FLEETDOCK_ADMIN_EMAIL\",\"password\":\"$FLEETDOCK_ADMIN_PASSWORD\"}" \
    "$BASE/v1/auth/login" | jq -er .token
}

echo "==> Starting $OLD_REF and creating data"
start "$WORK/api-old" old
TOKEN="$(login)"
api POST /v1/users '{"name":"Upgrade Reader","email":"reader@upgrade.test","password":"reader-password-1","role":"viewer"}' >/dev/null
API_TOKEN="$(api POST /v1/tokens '{"name":"upgrade-test","scopes":[]}' | jq -er .token)"
api POST /v1/notification-channels \
  '{"name":"upgrade-hook","type":"webhook","config":{"url":"https://hooks.example.com/upgrade"},"enabled":true}' >/dev/null
api POST /v1/backup-destinations \
  '{"name":"upgrade-s3","provider":"s3","bucket":"upgrade-bucket","region":"us-east-1","access_key_id":"AKIAUPGRADE","secret_access_key":"upgrade-secret"}' >/dev/null
stop

echo "==> Starting this checkout on the same database"
start "$WORK/api-new" new
TOKEN="$(login)"
fail() { echo "upgrade check failed: $*" >&2; cat "$WORK/new.log" >&2; exit 1; }

api GET /v1/users | jq -e '[.items // . | .[] | select(.email == "reader@upgrade.test")] | length == 1' >/dev/null ||
  fail "the user created before the upgrade is gone"
TOKEN="$API_TOKEN" api GET /v1/auth/me | jq -e '.email == "admin@upgrade.test"' >/dev/null ||
  fail "an API token created before the upgrade no longer works"
api GET /v1/notification-channels | jq -e '[.items // . | .[] | select(.name == "upgrade-hook")] | length == 1' >/dev/null ||
  fail "the notification channel is gone"
api GET /v1/backup-destinations | jq -e '[.items // . | .[] | select(.name == "upgrade-s3")] | length == 1' >/dev/null ||
  fail "the backup destination is gone"
api GET /v1/version | jq -e '.version' >/dev/null || fail "/v1/version is missing"
if command -v psql >/dev/null 2>&1; then
  # Channel secrets saved by the old version are encrypted on the first start.
  plain="$(psql "$DATABASE_URL" -Atc "SELECT count(*) FROM notification_channels WHERE config::text LIKE '%hooks.example.com%'")"
  [ "$plain" = 0 ] || fail "a notification channel's webhook URL is still stored in plain text"
fi
stop

echo "==> Restarting: the upgraded database must be accepted again"
start "$WORK/api-new" new-again
stop

echo "Upgrade from $OLD_REF passed."
