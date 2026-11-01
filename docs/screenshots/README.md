# Dashboard screenshots

The README, the docs and [fleetdock.dev](https://fleetdock.dev) use these
images. Every image is a real capture of the dashboard, taken from a demo
install with sample data. They are not mock-ups.

```text
light/<name>.png   light theme, 1440×900 at 2×
dark/<name>.png    dark theme, same shots
```

## Regenerate them

Re-capture before a release whenever the UI has changed:

```bash
scripts/screenshots.sh
```

This needs Docker and Node 20+. The script:

1. Builds the Fleetdock image from this checkout.
2. Starts the demo stack in `deploy/demo/`:
   - Fleetdock on `http://127.0.0.1:18080`
   - sample PostgreSQL, MariaDB and MySQL servers
   - S3-compatible storage

   The stack is its own Compose project, so it doesn't touch a real install
   on the same machine.
3. Seeds it with `scripts/demo/seed.mjs`: servers, 8 sample databases,
   backups and schedules, data edits with undo history, schema changes, saved
   queries, a fleet run, users, roles, an API token and alerts.
4. Captures every shot in both themes with Playwright
   (`frontend/e2e/screenshots/product.capture.ts`).
5. Copies the images to `../fleetdock-web/public/screenshots/` when the
   website repository sits next to this one.

Useful options:

| Option | Effect |
| --- | --- |
| `ONLY=overview,backups scripts/screenshots.sh` | Capture only some shots |
| `KEEP=1 scripts/screenshots.sh` | Leave the demo running afterwards (sign in as `admin@demo.fleetdock.dev` / `fleetdock-demo-password`) |
| `FRESH=1 scripts/screenshots.sh` | Wipe the demo data and start from scratch |

To remove the demo completely:

```bash
docker compose -f deploy/demo/docker-compose.yml down -v
```

## Adding a shot

Add an entry to `SHOTS` in `frontend/e2e/screenshots/product.capture.ts`.
If it needs data, seed that in `scripts/demo/seed.mjs`. Then list the new
name, with its alt text, in `fleetdock-web/components/shared/screenshot.tsx`
so the website can use it.
