# Glossary — words used in the dashboard

The dashboard speaks to people who run databases, not to people who built
Fleetdock. Use the words on the left in anything a user reads (labels, buttons,
empty states, errors, notifications). The right column is what the code and
the API call the same thing; keep using those names in code.

| Say | Not | Code / API name |
|---|---|---|
| Server | host, node, agent host | `server` |
| Database server | instance, cluster | `instance` |
| Database | schema (MySQL), DB | `database` |
| Created by Fleetdock | provisioned, docker | `provisioned` |
| On your server | managed | `kind: managed` |
| Connected remotely | external | `kind: external` |
| Not connected — add a login | metadata-only, no credentials | `has_credentials: false` |
| Not found on server | missing | `status: missing` |
| Activity, task | operation, job | `operation` / `jobs` |
| Run by Fleetdock / the agent | control plane / executor | `server_id` on an operation |
| Backup storage | destination | `destination` |
| Back up now | trigger backup | `POST /v1/backups` |
| Check backup ("restores OK") | verify | `verify_status` |
| Restore | — | `restore` |
| Copy or move | migrate | `migrate` / moves |
| Schedule, "how often" | cron job | `backup_schedules` |
| Keep backups for N days | retention | `retention_days` |
| Encryption | TLS mode | `tls_mode` |
| Connect through an SSH tunnel | SSH tunnel, bastion | `ssh_tunnel` |
| Host key ("Reset") | pinned SSH host key (TOFU) | `host_key_fingerprint`, `reset_ssh_host_key` |
| Database users / Access | users & grants | `db-users`, grants |
| API token | — | `api_tokens` |

## Style

- Say what happens, in the user's terms: "Removes the database and all its
  data", not "Runs DROP DATABASE".
- Destructive confirmations say what is lost and what is kept.
- Errors say what went wrong and, if possible, what to do next
  (`friendlyError` in `frontend/src/lib/errors.ts` maps API errors).
- Times are relative ("3 min ago") with the exact time on hover — use
  `<Time>`; cron schedules are always UTC and say so.
- Technical words that can't be avoided (port, bucket, cron) get a
  `help` tooltip on their form field.
