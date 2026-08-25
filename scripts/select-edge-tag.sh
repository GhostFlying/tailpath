#!/bin/sh
set -eu

ref="${1:?usage: select-edge-tag.sh REF TAGS_FILE}"
tags_file="${2:?usage: select-edge-tag.sh REF TAGS_FILE}"

test -r "$tags_file"

for revision in $(git rev-list "$ref"); do
  tag="edge-$revision"
  if grep -Fxq "$tag" "$tags_file"; then
    printf '%s\n' "$tag"
    exit 0
  fi
done

echo "no published edge image is reachable from $ref" >&2
exit 1
