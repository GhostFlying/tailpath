#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
helper="$root/scripts/exporter-dogfood.sh"
temporary=$(mktemp -d /tmp/tailpath-exporter-helper.XXXXXX)
evidence=$(mktemp -d /tmp/tailpath-exporter-dogfood-evidence.XXXXXX)
trap 'rm -rf "$temporary" "$evidence"' EXIT HUP INT TERM

if TAILPATH_EXPORTER_DOGFOOD_PROJECT=unsafe "$helper" status >/dev/null 2>&1; then
  echo "exporter dogfood accepted an unsafe project" >&2
  exit 1
fi
if TAILPATH_VERSION=edge-short "$helper" up >/dev/null 2>&1; then
  echo "exporter dogfood accepted a short immutable tag" >&2
  exit 1
fi
if "$helper" capture 'unsafe scenario' >/dev/null 2>&1; then
  echo "exporter dogfood accepted capture without validated runtime state" >&2
  exit 1
fi

runtime_file="$temporary/runtime.env"
cat >"$runtime_file" <<EOF
TAILPATH_VERSION=edge-0123456789abcdef0123456789abcdef01234567
TAILPATH_EXPORTER_DOGFOOD_PREFIX=tailpath-exporter-dogfood
TAILPATH_EXPORTER_DOGFOOD_EVIDENCE=$evidence
TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE=/tmp/tailpath-exporter-dogfood-authkey.test
EOF
for side in before after; do
  cat >"$evidence/$side-topology.raw.json" <<'EOF'
{"nodes":[{"id":"a","hostname":"tailpath-exporter-dogfood-runtime-a"},{"id":"b","hostname":"tailpath-exporter-dogfood-runtime-b"}],"edges":[{"id":"edge-private","source":"a","target":"b"}]}
EOF
done
cat >"$evidence/before-topology.json" <<'EOF'
{"businessEdge":{"bytesPerSecond":1000,"forwardPositive":true,"reversePositive":true}}
EOF
cat >"$evidence/after-topology.json" <<'EOF'
{"businessEdge":{"bytesPerSecond":3000,"forwardPositive":true,"reversePositive":true}}
EOF
cat >"$evidence/after-history.json" <<'EOF'
{"trafficPoints":1,"directionalTraffic":{"forwardPositive":true,"reversePositive":true}}
EOF
TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" \
  "$helper" assert-continuity before after >/dev/null

jq '.businessEdge.bytesPerSecond = 20000000' "$evidence/after-topology.json" >"$temporary/spike.json"
mv "$temporary/spike.json" "$evidence/after-topology.json"
if TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" \
  "$helper" assert-continuity before after >/dev/null 2>&1; then
  echo "exporter dogfood accepted a catch-up rate spike" >&2
  exit 1
fi

touch "$evidence/private.raw.json" "$evidence/private.raw.log" "$evidence/safe.json"
TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" "$helper" purge-raw >/dev/null
test ! -e "$evidence/private.raw.json"
test ! -e "$evidence/private.raw.log"
test -e "$evidence/safe.json"
