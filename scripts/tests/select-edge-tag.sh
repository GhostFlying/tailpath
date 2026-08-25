#!/bin/sh
set -eu

repo_root="$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

fixture="$tmpdir/repository"
git init -q "$fixture"
export GIT_AUTHOR_NAME="Tailpath fixture"
export GIT_AUTHOR_EMAIL="fixture@tailpath.invalid"
export GIT_AUTHOR_DATE="2000-01-01T00:00:00Z"
export GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME"
export GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"
export GIT_COMMITTER_DATE="$GIT_AUTHOR_DATE"

empty_tree="$(git -C "$fixture" mktree < /dev/null)"
parent_revision="$(printf 'parent\n' | git -C "$fixture" commit-tree "$empty_tree")"
head_revision="$(printf 'head\n' | git -C "$fixture" commit-tree "$empty_tree" -p "$parent_revision")"
git -C "$fixture" update-ref refs/heads/main "$head_revision"

printf 'edge-%s\nedge-%s\n' "$parent_revision" "$head_revision" > "$tmpdir/tags"
selected="$(cd "$fixture" && "$repo_root/scripts/select-edge-tag.sh" main "$tmpdir/tags")"
test "$selected" = "edge-$head_revision"

printf 'unrelated\nedge-%s\n' "$parent_revision" > "$tmpdir/tags"
selected="$(cd "$fixture" && "$repo_root/scripts/select-edge-tag.sh" main "$tmpdir/tags")"
test "$selected" = "edge-$parent_revision"

printf 'edge-%040d\n' 0 > "$tmpdir/tags"
if (cd "$fixture" && "$repo_root/scripts/select-edge-tag.sh" main "$tmpdir/tags") > /dev/null 2>&1; then
  echo "selector accepted a tag outside the current history" >&2
  exit 1
fi
