# Releasing db-manager

How maintainers cut a new version, publish container images, and verify the result.

## Versioning

This project follows [Semantic Versioning](https://semver.org/):

- **MAJOR** — breaking API or migration changes
- **MINOR** — new features, backward compatible
- **PATCH** — bug fixes, docs, CI only

Tags use a `v` prefix: `v0.1.0`, `v0.1.1`, `v0.2.0`.

## Release checklist

1. Merge all changes for the release into `main`
2. Update **[CHANGELOG.md](CHANGELOG.md)** — move `[Unreleased]` entries under a new version heading with today's date
3. Open a PR if the changelog commit is not on `main` yet
4. Create and push the tag (see below)
5. Wait for the **[Release workflow](.github/workflows/release.yml)** to finish on GitHub Actions
6. Verify the [GitHub Release](https://github.com/TajBrains/db-manager/releases) and GHCR images
7. Optionally announce (release notes, blog, social)

## Tag and push

```bash
git checkout main
git pull origin main

# After CHANGELOG.md is updated:
git add CHANGELOG.md
git commit -m "chore: prepare CHANGELOG for v0.1.1"
git push origin main

git tag -a v0.1.1 -m "v0.1.1"
git push origin v0.1.1
```

To re-run a failed release, delete the remote tag and push again:

```bash
git push origin :refs/tags/v0.1.1
git tag -fa v0.1.1 -m "v0.1.1"
git push origin v0.1.1
```

## What the release workflow does

On every `v*` tag push:

1. Builds and pushes Docker images to GHCR (lowercase org name):
   - `ghcr.io/tajbrains/db-manager-backend:<tag>` and `:latest`
   - `ghcr.io/tajbrains/db-manager-frontend:<tag>` and `:latest`
2. Cross-compiles Linux agent binaries (`amd64`, `arm64`) and API binary (`amd64`)
3. Creates a GitHub Release with tarballs and `SHA256SUMS`
4. Attaches changelog section for that version as release notes (when present in CHANGELOG.md)

## Frontend build URL

The frontend image bakes in `NEXT_PUBLIC_API_URL` at **build time**.

Set a GitHub repository variable before tagging if the default `http://localhost:8080` is wrong for your users:

```bash
gh variable set NEXT_PUBLIC_API_URL --body "https://dbm.example.com"
```

Re-tag or cut a new patch release after changing this variable so the frontend image is rebuilt.

## GHCR visibility

Packages are private by default. To allow unauthenticated `docker pull`:

1. GitHub → **Packages** → `db-manager-backend` / `db-manager-frontend`
2. **Package settings** → **Change visibility** → Public

## Smoke-test a release

```bash
export TAG=v0.1.1
docker pull ghcr.io/tajbrains/db-manager-backend:${TAG}
docker pull ghcr.io/tajbrains/db-manager-frontend:${TAG}

# Or use the GHCR compose overlay:
MDCP_RELEASE_TAG=${TAG} docker compose -f docker-compose.yml -f docker-compose.ghcr.yml up -d

curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
```

Sign in at http://localhost:3000 with bootstrap credentials from `.env`.

## Hotfix workflow

1. Branch from the release tag (or `main` if it still matches production)
2. Fix, test, merge to `main`
3. Bump **PATCH** version in CHANGELOG
4. Tag `v0.1.2` (etc.) and push

Do not move an existing tag to a different commit once users may have pulled it.
