# Dashboard screenshots

Screenshots in this directory are used by the README and release pages.

## Current status

Replace generated or outdated images with **real captures** from a running stack
before major announcements.

## How to capture

```bash
cp .env.example .env
./scripts/generate-secrets.sh >> .env
docker compose up --build -d
# Dashboard: http://localhost:3000 — log in with MDCP_ADMIN_* from .env
```

Suggested files:

| File | Page |
|------|------|
| `dashboard-overview.png` | Overview (home) |
| `servers.png` | Servers list |
| `backups.png` | Backups or Operations |

Use your OS screenshot tool or browser devtools (responsive 1280×720 works well for README).
