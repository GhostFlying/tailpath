#!/bin/sh
set -eu

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

cp api/generated/types.gen.go "$tmpdir/types.gen.go"
cp web/src/api/schema.ts "$tmpdir/schema.ts"
make generate
cmp api/generated/types.gen.go "$tmpdir/types.gen.go"
cmp web/src/api/schema.ts "$tmpdir/schema.ts"
