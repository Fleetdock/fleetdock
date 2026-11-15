# Releasing Fleetdock

How maintainers cut a new version, publish container images, and verify the result.

## Versioning

This project follows [Semantic Versioning](https://semver.org/):

- **MAJOR** — breaking API or migration changes
- **MINOR** — new features, backward compatible
- **PATCH** — bug fixes, docs, CI only

Tags use a `v` prefix: `v0.10.0`, `v0.10.1`, `v0.11.0`. Release candidates are
`v0.11.0-rc.1`: they are published as GitHub pre-releases and do not move the
`latest` image tag.

## Compatibility

Fleetdock is still before 1.0, so under SemVer a minor release (0.11, 0.12, …)
may change or remove things. From 0.10 the project follows the rules below
anyway; 1.0 turns them into a promise for the whole 1.x line.

**HTTP API (`/v1`).** No endpoint, field, query parameter or error code
documented in [openapi.yaml](backend/internal/openapi/openapi.yaml) is removed
or renamed in a patch release, and no field changes type or meaning. A minor
release may remove or change one, only after deprecating it (below) where at
all possible, and lists it under *Removed* or *Changed* in the changelog. New
endpoints, optional request fields, response fields and enum values may be
added at any time, so clients must ignore fields they do not know. Errors keep
the `{error: {code, message, field}}` envelope; match on `code` and the HTTP
status, never on `message`. Undocumented routes (the dashboard's own, like
`/healthz` details) are not covered.

**Deprecation.** Something that will go is marked `deprecated: true` in the
spec, answers with a `Deprecation` header (and `Sunset` once a date is set),
and is announced under *Deprecated* in the changelog at least one minor
release before the release that removes it.

**Agents (`/agent/v1`).** A control plane accepts agents from its own minor
version and the one before (`min_agent_version` in `GET /v1/version`), so the
fleet can be updated after the control plane. Older agents are flagged in the
dashboard and should be updated; see [OPERATIONS.md](docs/OPERATIONS.md#updating-agents).

**Configuration and CLI.** A `FLEETDOCK_*` variable or `fleetdock` command
that is replaced keeps working, with a warning, for at least one minor release.

**Upgrades and migrations.** Every release upgrades in place from the previous
minor release and its patches. Migrations only move forward; to go back,
restore the metadata backup taken before the upgrade (`fleetdock update` takes
one). CI upgrades the latest release to every commit
(`scripts/ci/upgrade-test.sh`).

**Supported versions.** The latest minor release gets fixes; security fixes
also go to the previous minor for three months after a new one ships.

## Release checklist

1. Merge all changes for the release into `main`
2. Run `make release-prep VERSION=0.11.0`: it moves the `[Unreleased]` entries under `## [0.11.0] - <today>` and updates the compare links at the bottom of **[CHANGELOG.md](CHANGELOG.md)**. Read the section through, then open a PR with it and merge it
3. Tag the merge commit of that PR (below). Do not create the release in GitHub's *Draft a new release* page: the workflow creates it from the tag, with this version's CHANGELOG section as its notes
4. For a minor or major release, tag a release candidate first (`v0.11.0-rc.1`): it is published as a pre-release and leaves `latest` alone. Install it on a fresh VM with `install.sh --tag v0.11.0-rc.1`, upgrade a copy of a real install to it, enrol and update an agent, then go on
5. Create and push the tag (see below)
6. Wait for the **[Release workflow](.github/workflows/release.yml)** to finish on GitHub Actions: it runs the whole CI suite first
7. Verify the [GitHub Release](https://github.com/Fleetdock/fleetdock/releases) and GHCR images: `/api version` in the image prints the tag, and the signatures verify (below)
8. Optionally announce (release notes, blog, social)

## Tag and push

```bash
git checkout main
git pull origin main

# On a branch: prepare the changelog, then open a PR with it and merge it.
git checkout -b release-0.10.1
make release-prep VERSION=0.10.1
git commit -am "Prepare CHANGELOG for v0.10.1"
git push origin release-0.10.1

# After the PR is merged, tag the merge commit:
git checkout main
git pull origin main
git tag -a v0.10.1 -m "Fleetdock 0.10.1"
git push origin v0.10.1
```

To re-run a failed release, delete the remote tag and push again:

```bash
git push origin :refs/tags/v0.10.1
git tag -fa v0.10.1 -m "v0.10.1"
git push origin v0.10.1
```

## What the release workflow does

On every `v*` tag push:

1. Builds and pushes **one** multi-arch image to GHCR (lowercase org name):
   `ghcr.io/fleetdock/fleetdock` for `linux/amd64` and `linux/arm64`, tagged
   `v<tag>`, `<tag>`, `<major>.<minor>` and `latest`.
2. Verifies the image boots: node version, dashboard bundle present, agent
   binaries executable.
3. Signs the image with cosign (keyless), and cross-compiles the Linux agent
   and API binaries (`amd64`, `arm64`), stamped with the version
4. Creates a GitHub Release with tarballs, `SHA256SUMS`, and the installer
   artefacts (`install.sh`, `fleetdock`, `docker-compose.yml`) so a release can
   be installed from a pinned tag
5. Uses that version's CHANGELOG.md section as the release notes (`scripts/changelog.sh notes`), with relative links made absolute

Nothing deployment-specific is baked into the image. There is no
`NEXT_PUBLIC_API_URL` build argument any more: the dashboard calls the API on
whatever origin serves it, so one image works for every self-hoster.

## Verifying a release

Images and checksum files are signed with [cosign](https://docs.sigstore.dev/)
keyless signing: the signature proves they were built by this repository's
release workflow from a `v*` tag. Image and SBOM/provenance attestations are
attached to the image too.

```bash
cosign verify ghcr.io/fleetdock/fleetdock:v0.10.0 \
  --certificate-identity-regexp '^https://github.com/Fleetdock/fleetdock/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

cosign verify-blob SHA256SUMS --bundle SHA256SUMS.sigstore.json \
  --certificate-identity-regexp '^https://github.com/Fleetdock/fleetdock/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum -c SHA256SUMS --ignore-missing
```

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
export TAG=v0.10.1
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

## Patch releases of Go

The Go version is set in one place: the `toolchain` line in `backend/go.mod`.
CI and the release workflow install exactly that version (`setup-go` with
`go-version-file`). The image builds with `golang:1.26-alpine`, which always
has the latest 1.26 patch.

Dependabot does not update the `toolchain` line: it only bumps modules in
`require`. When `govulncheck` in CI reports a vulnerability in the standard
library (`Found in: net/http@go1.26.x`, `Fixed in: …`), raise `toolchain` to
the fixed release, note it under *Security* in the changelog, and consider a
patch release.

## Hotfix workflow

1. Branch from the release tag (or `main` if it still matches production)
2. Fix, test, merge to `main`
3. Bump **PATCH** version in CHANGELOG
4. Tag `v0.10.2` (etc.) and push

Do not move an existing tag to a different commit once users may have pulled it.
