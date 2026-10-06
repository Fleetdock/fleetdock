# Install on your own computer

Fleetdock can run entirely on your own machine — macOS, Linux, or Windows
through WSL2 — with nothing reachable from the network. Your metadata,
credentials and encryption key never leave the computer.

This is the right choice when:

- you want to manage databases you can already reach from this computer (on
  it, on your network, or through a VPN or SSH tunnel), and
- you do not need other servers to connect to Fleetdock.

If you want to manage a fleet of servers with the Fleetdock agent, install on a
server instead — see the [Quick start](../README.md#quick-start).

## 1. Install Docker

| Your computer | What to install |
| --- | --- |
| **macOS** | [Docker Desktop](https://docker.com/products/docker-desktop) (or `brew install --cask docker`). Start it once. |
| **Windows** | [Docker Desktop](https://docker.com/products/docker-desktop) and WSL2 (`wsl --install` in PowerShell). In Docker Desktop: **Settings → Resources → WSL integration**, turn it on for your Linux distribution (Ubuntu by default). |
| **Linux** | [Docker Engine](https://docs.docker.com/engine/install/). To run without `sudo`, add yourself to the docker group: `sudo usermod -aG docker "$USER"`, then log out and back in. |

## 2. Run the installer

Open a terminal — on Windows, the **Ubuntu (WSL)** terminal, not PowerShell or
Git Bash — and run:

```bash
curl -sSL https://fleetdock.dev/install.sh | sh -s -- --local
```

On macOS and WSL `--local` is the default, so the plain command works too. On
Linux, the installer asks where you are installing; answer **2) Only this
computer**.

It shows what it is about to do and asks you to confirm, then prints your login:

```text
  Fleetdock is running.

    Dashboard   http://localhost
    Email       admin@example.com
    Password    8fK2mQ...
```

Open the address in your browser. On Windows, use your normal Windows browser —
Docker Desktop forwards `localhost` from WSL.

**Port 80 already taken?** Pick another port:

```bash
curl -sSL https://fleetdock.dev/install.sh | sh -s -- --local --port 8090
```

The dashboard is then at `http://localhost:8090`.

## 3. Connect your databases

In the dashboard: **Databases → Connect database server → Connect to one
anywhere**, then enter the host, port and an admin login.

| Where the database runs | Host to enter |
| --- | --- |
| On this same computer (installed directly, or in its own Docker container with a published port) | `host.docker.internal` |
| Another machine on your network | its IP address or hostname, e.g. `192.168.1.20` |
| Behind an SSH tunnel you opened on this computer | `host.docker.internal` and the tunnel's local port |
| A hosted database (RDS, Neon, …) | its hostname, as your provider shows it |

Do **not** enter `localhost` or `127.0.0.1`: Fleetdock runs in a container, so
those mean the container itself, and Fleetdock refuses them.

A database running on this computer must listen on an address Docker can reach.
PostgreSQL in particular often listens on `127.0.0.1` only — set
`listen_addresses = '*'` (or the Docker bridge address) in `postgresql.conf`
and allow the Docker network in `pg_hba.conf`. A database in its own Docker
container just needs its port published (`-p 5432:5432`).

## Where everything is

| What | Where |
| --- | --- |
| Configuration and keys | `~/.fleetdock/.env` (or `/opt/fleetdock/.env` if you installed with sudo) |
| Metadata (servers, instances, users, backups list) | the `fleetdock_postgres_data` Docker volume |
| The `fleetdock` command | `~/.local/bin/fleetdock` (or `/usr/local/bin/fleetdock`) |

Back up the configuration and the metadata now and then, and keep the copies off
this computer:

```bash
fleetdock backup-config   # .env — holds the key that decrypts stored passwords
fleetdock backup-db       # the metadata database
```

Without `.env`, a metadata backup is unreadable: stored passwords are encrypted
with `FLEETDOCK_ENCRYPTION_KEY`.

## What a local install cannot do

| Limit | Why |
| --- | --- |
| Servers elsewhere cannot connect their agents | They need to reach Fleetdock, and `localhost` is only reachable from this computer. Provisioning and agent-run backups need agents. |
| Scheduled backups and alerts run only while the computer is on | They are run by Fleetdock itself, which sleeps with your laptop |
| No HTTPS | There is no public address for a certificate. Traffic never leaves the computer, so this is fine while it stays bound to `localhost` |
| External database access (the gateway) | Docker Desktop hides the real client address, so its allowlists reject everyone |

### Letting machines on your network in

To let agents on the same network (or a VPN such as Tailscale) enrol, give
Fleetdock an address they can reach:

```bash
fleetdock domain 192.168.1.20      # your computer's LAN or VPN address
```

This also makes Fleetdock listen on every network interface — the dashboard is
then reachable, over plain HTTP, by anyone who can reach that address. Go back
with `fleetdock domain localhost`.

## Moving to a server later

1. On this computer: `fleetdock backup-db`, and note `FLEETDOCK_ENCRYPTION_KEY`
   from `fleetdock env --reveal`. Copy the backup file to the server.
2. On the server: install normally (see the [Quick start](../README.md#quick-start)).
3. Replace the new, empty metadata with yours:

   ```bash
   fleetdock stop fleetdock
   fleetdock psql -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
   gunzip -c metadata-….sql.gz | fleetdock psql
   ```

4. Use your old encryption key — without it the stored passwords cannot be
   read — and start again:

   ```bash
   FLEETDOCK_I_UNDERSTAND=1 fleetdock config set FLEETDOCK_ENCRYPTION_KEY '<your old key>'
   fleetdock start
   ```

Sign in with the account you used locally; the server's generated admin
password no longer applies.

## Removing it

```bash
fleetdock delete
```

It lists what it will remove and asks you to type `DELETE`.
