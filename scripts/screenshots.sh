#!/usr/bin/env bash
# Capture the product screenshots in docs/screenshots/{light,dark}/ from a
# freshly built, seeded demo install. Re-run it before a release so the README,
# the docs and fleetdock.dev show the current UI.
#
#   scripts/screenshots.sh            # build, start, seed, capture, copy to the website
#   ONLY=overview,backups scripts/screenshots.sh
#   KEEP=1 scripts/screenshots.sh     # leave the demo stack running afterwards
#   FRESH=1 scripts/screenshots.sh    # wipe the demo data and start over
#
# The demo stack (deploy/demo/docker-compose.yml) is its own compose project
# bound to 127.0.0.1:18080, so it never touches a real install on this machine.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
compose=(docker compose -f "$root/deploy/demo/docker-compose.yml")
port="${FLEETDOCK_DEMO_PORT:-18080}"
url="http://127.0.0.1:${port}"
web="${FLEETDOCK_WEB_DIR:-$root/../fleetdock-web}"

step() { printf '\n==> %s\n' "$*"; }

if [[ "${FRESH:-}" == "1" ]]; then
  step "Removing the old demo stack and its data"
  "${compose[@]}" down -v
fi

step "Building the Fleetdock image from this checkout"
"${compose[@]}" build fleetdock

step "Starting the demo stack on ${url}"
"${compose[@]}" up -d --wait

step "Seeding demo data"
FLEETDOCK_DEMO_URL="$url" node "$root/scripts/demo/seed.mjs"

step "Capturing screenshots (light and dark)"
cd "$root/frontend"
if ! npx --no-install playwright --version >/dev/null 2>&1; then npm ci; fi
npx playwright install chromium >/dev/null
FLEETDOCK_SHOTS_URL="$url" ONLY="${ONLY:-}" npm run --silent screenshots

if [[ -d "$web/public" ]]; then
  step "Copying screenshots to ${web}/public/screenshots"
  mkdir -p "$web/public/screenshots"
  cp -R "$root/docs/screenshots/light" "$root/docs/screenshots/dark" "$web/public/screenshots/"
fi

if [[ "${KEEP:-}" != "1" ]]; then
  step "Stopping the demo stack (data kept; FRESH=1 wipes it)"
  "${compose[@]}" stop
fi

step "Done: $(find "$root/docs/screenshots" -name '*.png' | wc -l | tr -d ' ') screenshots in docs/screenshots/"
