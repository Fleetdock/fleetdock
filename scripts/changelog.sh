#!/bin/sh
# Release helpers for CHANGELOG.md (Keep a Changelog format).
#
#   scripts/changelog.sh prepare 0.11.0
#       Turns the [Unreleased] entries into "## [0.11.0] - <today>" and updates
#       the compare links at the bottom. Commit the result in a PR, merge it,
#       then tag the merge commit (see RELEASING.md).
#
#   scripts/changelog.sh notes v0.11.0
#       Prints that version's section, with links made absolute so they work
#       on the GitHub release page. The release workflow uses this.
#
#   scripts/changelog.sh previous v0.11.0
#       Prints the release before it (0.10.0), or nothing for the first one.
#       The release workflow uses it to list the release's contributors.
#
# REPO (owner/name) and CHANGELOG (path) can be set in the environment.
set -eu

REPO="${REPO:-Fleetdock/fleetdock}"
CHANGELOG="${CHANGELOG:-CHANGELOG.md}"

die() {
  echo "changelog.sh: $*" >&2
  exit 1
}

valid_version() {
  echo "$1" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$'
}

# version_gt A B: true when A is newer than B (pre-releases sort before the release).
version_gt() {
  [ "$1" != "$2" ] || return 1
  highest="$(printf '%s\n%s\n' "$1" "$2" | sed '/-/!s/$/_/' | sort -V | tail -n1 | sed 's/_$//')"
  [ "$highest" = "$1" ]
}

prepare() {
  ver="${1#v}"
  valid_version "$ver" || die "version must look like 1.2.3 or 1.2.3-rc.1, got '$1'"
  [ -f "$CHANGELOG" ] || die "$CHANGELOG not found"
  grep -q '^## \[Unreleased\]' "$CHANGELOG" || die "no '## [Unreleased]' heading in $CHANGELOG"
  if grep -q "^## \[$ver\]" "$CHANGELOG"; then
    die "$CHANGELOG already has a section for $ver"
  fi

  # The newest released version: the first "## [x.y.z]" heading.
  prev="$(sed -n 's/^## \[\([0-9][^]]*\)\].*/\1/p' "$CHANGELOG" | head -n1)"
  [ -n "$prev" ] || die "no released version found in $CHANGELOG"
  version_gt "$ver" "$prev" || die "$ver is not newer than the latest release, $prev"

  # Something must be listed under [Unreleased].
  entries="$(awk '/^## \[/ { if (p) exit; if ($0 ~ /^## \[Unreleased\]/) { p = 1; next } } p && /^- /' "$CHANGELOG")"
  [ -n "$entries" ] || die "[Unreleased] has no entries: nothing to release"

  today="$(date -u +%Y-%m-%d)"
  tmp="$(mktemp)"
  awk -v ver="$ver" -v prev="$prev" -v today="$today" -v repo="$REPO" '
    /^## \[Unreleased\]/ && !h { print; print ""; print "## [" ver "] - " today; h = 1; next }
    /^\[Unreleased\]: / {
      print "[Unreleased]: https://github.com/" repo "/compare/v" ver "...HEAD"
      print "[" ver "]: https://github.com/" repo "/compare/v" prev "...v" ver
      next
    }
    { print }
  ' "$CHANGELOG" >"$tmp"
  mv "$tmp" "$CHANGELOG"

  echo "Prepared $CHANGELOG for $ver (previous release: $prev)."
  echo "Next:"
  echo "  1. Review the section, open a PR with it and merge it."
  echo "  2. Tag the merge commit and push the tag:"
  echo "       git tag -a v$ver -m \"Fleetdock $ver\" <merge commit>"
  echo "       git push origin v$ver"
  echo "  The release workflow publishes the release with these notes."
}

notes() {
  tag="$1"
  ver="${tag#v}"
  [ -f "$CHANGELOG" ] || die "$CHANGELOG not found"
  # This version's section only: from its heading to the next "## [".
  # Relative links ("](docs/X.md#a)") become absolute links to the tagged
  # tree; http(s), mailto and in-page anchors are left as they are.
  awk -v ver="$ver" '
    /^## \[/ { if (p) exit; if (index($0, "## [" ver "]") == 1) { p = 1; next } }
    p
  ' "$CHANGELOG" |
    sed -E "s|\]\(([^)/:#][^):]*)\)|](https://github.com/$REPO/blob/$tag/\1)|g"
}

previous() {
  ver="${1#v}"
  [ -f "$CHANGELOG" ] || die "$CHANGELOG not found"
  # The first released heading after this version's.
  awk -v ver="$ver" '
    /^## \[/ {
      if (seen) { sub(/^## \[/, ""); sub(/\].*/, ""); print; exit }
      if (index($0, "## [" ver "]") == 1) seen = 1
    }
  ' "$CHANGELOG" | {
    # Old headings such as "0.8.1 – 0.8.5" are not a single release.
    read -r prev || true
    if [ -n "${prev:-}" ] && valid_version "$prev"; then echo "$prev"; fi
  }
}

case "${1:-}" in
  prepare)
    [ $# -eq 2 ] || die "usage: $0 prepare <version>"
    prepare "$2"
    ;;
  notes)
    [ $# -eq 2 ] || die "usage: $0 notes <tag>"
    notes "$2"
    ;;
  previous)
    [ $# -eq 2 ] || die "usage: $0 previous <tag>"
    previous "$2"
    ;;
  *)
    die "usage: $0 prepare <version> | notes <tag> | previous <tag>"
    ;;
esac
