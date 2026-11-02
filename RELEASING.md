# Releasing Fleetdock

How maintainers cut a new version, publish container images, and verify the result.

## Versioning

This project follows [Semantic Versioning](https://semver.org/):

- **MAJOR** — breaking API or migration changes
- **MINOR** — new features, backward compatible
- **PATCH** — bug fixes, docs, CI only

Tags use a `v` prefix: `v1.0.0`, `v1.0.1`, `v1.1.0`. Release candidates are
`v1.1.0-rc.1`: they are published as GitHub pre-releases and do not move the
`latest` image tag.

## Compatibility promise (from 1.0)

What a minor or patch release will not break, and how anything that has to go
is retired.

**HTTP API (`/v1`).** Within 1.x, no endpoint, field, query parameter or
error code documented in [openapi.yaml](backend/internal/openapi/openapi.yaml)
is removed or renamed, and no field changes type or meaning. New endpoints,
optional request fields, response fields and enum values may be added at any
time, so clients must ignore fields they do not know. Errors keep the
`{error: {code, message, field}}` envelope; match on `code` and the HTTP
status, never on `message`. Undocumented routes (the dashboard's own, like
`/healthz` details) are not covered.

**Deprecation.** Something that will go is marked `deprecated: true` in the
spec, answers with a `Deprecation` header (and `Sunset` once a date is set),
and is announced under *Deprecated* in the changelog at least one minor
release before the major release that removes it.

**Agents (`/agent/v1`).** A control plane accepts agents from its own minor
version and the one before (`min_agent_version` in `GET /v1/version`), so the
fleet can be updated after the control plane. Older agents are flagged in the
dashboard and should be updated; see [OPERATIONS.md](docs/OPERATIONS.md#updating-agents).

**Configuration and CLI.** `FLEETDOCK_*` variables and `fleetdock` commands
keep working within 1.x; one that is replaced keeps working, with a warning,
until the next major release.

**Upgrades and migrations.** Any 1.x release upgrades in place from any
earlier 1.x release, and from the last 0.9 release. Migrations only move
forward; to go back, restore the metadata backup taken before the upgrade
(`fleetdock update` takes one). CI upgrades the latest release to every
commit (`scripts/ci/upgrade-test.sh`).

**Supported versions.** The latest minor release gets fixes; security fixes
also go to the previous minor for three months after a new one ships.

## Release checklist

1. Merge all changes for the release into `main`
2. Update **[CHANGELOG.md](CHANGELOG.md)** — move `[Unreleased]` entries under a new version heading with today's date
3. Open a PR if the changelog commit is not on `main` yet
4. Create and push the tag (see below)
5. Wait for the **[Release workflow](.github/workflows/release.yml)** to finish on GitHub Actions
6. Verify the [GitHub Release](https://github.com/Fleetdock/fleetdock/releases) and GHCR images
7. Optionally announce (release notes, blog, social)

## Tag and push

```bash
git checkout main
git pull origin main

# After CHANGELOG.md is updated:
git add CHANGELOG.md
git commit -m "chore: prepare CHANGELOG for v1.0.1"
git push origin main

git tag -a v1.0.1 -m "v1.0.1"
git push origin v1.0.1
```

To re-run a failed release, delete the remote tag and push again:

```bash
git push origin :refs/tags/v1.0.1
git tag -fa v1.0.1 -m "v1.0.1"
git push origin v1.0.1
```

## What the release workflow does

On every `v*` tag push:

1. Builds and pushes **one** multi-arch image to GHCR (lowercase org name):
   `ghcr.io/fleetdock/fleetdock` for `linux/amd64` and `linux/arm64`, tagged
   `v<tag>`, `<tag>`, `<major>.<minor>` and `latest`.
2. Verifies the image boots: node version, dashboard bundle present, agent
   binaries executable.
3. Cross-compiles Linux agent binaries (`amd64`, `arm64`) and the API binary (`amd64`)
4. Creates a GitHub Release with tarballs, `SHA256SUMS`, and the installer
   artefacts (`install.sh`, `fleetdock`, `docker-compose.yml`) so a release can
   be installed from a pinned tag
5. Attaches the changelog section for that version as release notes (when present in CHANGELOG.md)

Nothing deployment-specific is baked into the image. There is no
`NEXT_PUBLIC_API_URL` build argument any more: the dashboard calls the API on
whatever origin serves it, so one image works for every self-hoster.

## GHCR visibility

Packages are private by default. To allow unauthenticated `docker pull`:

1. GitHub → **Packages** → `fleetdock`
2. **Package settings** → **Change visibility** → Public

## Publishing the installer

`https://fleetdock.dev/install.sh` is served from `fleetdock-web/public/install.sh`.
Refresh it from the canonical copy in this repo before deploying the site:

```bash
cd ../fleetdock-web && npm run sync:install
```

## Smoke-test a release

```bash
export TAG=v1.0.1
docker pull ghcr.io/fleetdock/fleetdock:${TAG}

cd /tmp && mkdir -p fd && cd fd
cp /path/to/fleetdock/docker-compose.yml .
cp /path/to/fleetdock/.env.example .env
/path/to/fleetdock/scripts/generate-secrets.sh >> .env
FLEETDOCK_RELEASE_TAG=${TAG} docker compose up -d

curl -fsS http://localhost/healthz
curl -fsS http://localhost/readyz
```

Sign in at `http://localhost` with the bootstrap credentials from `.env`. On a
real host, prefer the actual install path:
`curl -sSL https://fleetdock.dev/install.sh | sh -s -- --tag ${TAG}`.

## Hotfix workflow

1. Branch from the release tag (or `main` if it still matches production)
2. Fix, test, merge to `main`
3. Bump **PATCH** version in CHANGELOG
4. Tag `v1.0.2` (etc.) and push

Do not move an existing tag to a different commit once users may have pulled it.
