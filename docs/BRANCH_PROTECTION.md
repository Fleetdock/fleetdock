# Branch protection for `main`

The repository defines a [ruleset](.github/rulesets/protect-main.json) that:

| Rule | Effect |
|------|--------|
| **Pull request required** | No direct pushes to `main` (0 approvals required — fine for solo maintainers) |
| **Required checks** | `backend`, `frontend`, `docker` (CI workflow jobs) must pass |
| **Up to date** | Branch must be rebased/merged with latest `main` before merge |
| **No force-push** | History on `main` cannot be rewritten |
| **No deletion** | `main` cannot be deleted |

## Apply automatically

```bash
./scripts/setup-branch-protection.sh
```

Dry run (print JSON only):

```bash
./scripts/setup-branch-protection.sh --dry-run
```

Requires `gh` logged in with **admin** access to the repository.

## GitHub plan requirement

**Private repositories on GitHub Free** cannot use rulesets or classic branch
protection via the API. You need one of:

- [GitHub Pro](https://github.com/pricing) (personal private repos), or
- **Team / Enterprise** (organization repos), or
- Make the repository **public** (rulesets are free for public repos)

If the script fails with HTTP 403, use the manual UI steps below.

## Manual setup (GitHub UI)

1. Open **Settings → Rules → Rulesets**  
   `https://github.com/TajBrains/db-manager/settings/rules`
2. **New ruleset → Import a ruleset**
3. Upload [`.github/rulesets/protect-main.json`](../.github/rulesets/protect-main.json)
4. Save (enforcement should be **Active**)

### Classic branch protection (alternative)

If your plan only supports classic protection:

1. **Settings → Branches → Add branch ruleset** (or Branch protection rule)
2. Branch name pattern: `main`
3. Enable:
   - Require a pull request before merging
   - Require status checks to pass: `backend`, `frontend`, `docker`
   - Require branches to be up to date before merging
   - Do not allow bypassing the above settings
   - Block force pushes
   - Restrict deletions

## Adjusting rules

Edit `.github/rulesets/protect-main.json`, then re-run the setup script or
re-import in the UI.

To require a reviewer before merge (teams), set
`required_approving_review_count` to `1` in the `pull_request` rule.

## Verify

After applying, try pushing directly to `main` — it should be rejected. Merges
should only succeed when all three CI jobs are green.
