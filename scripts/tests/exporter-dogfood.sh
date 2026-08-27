#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
helper="$root/scripts/exporter-dogfood.sh"
temporary=$(mktemp -d /tmp/tailpath-exporter-helper.XXXXXX)
evidence=$(mktemp -d /tmp/tailpath-exporter-dogfood-evidence.XXXXXX)
runtime_file=$(mktemp /tmp/tailpath-exporter-dogfood-runtime.XXXXXX)
secret_dir=$(mktemp -d /tmp/tailpath-exporter-dogfood-secret.XXXXXX)
auth_file=$secret_dir/authkey
install -m 0444 /dev/null "$auth_file"
trap 'rm -rf "$temporary" "$evidence" "$secret_dir"; rm -f "$runtime_file"' EXIT HUP INT TERM

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

cat >"$runtime_file" <<EOF
TAILPATH_VERSION=edge-0123456789abcdef0123456789abcdef01234567
TAILPATH_EXPORTER_DOGFOOD_PREFIX=tailpath-exporter-dogfood
TAILPATH_EXPORTER_DOGFOOD_EVIDENCE=$evidence
TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE=$auth_file
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
cat >"$evidence/before-history.raw.json" <<'EOF'
{"traffic":[{"aToBBytes":1000,"bToABytes":1000}]}
EOF
cat >"$evidence/after-history.raw.json" <<'EOF'
{"traffic":[{"aToBBytes":2000,"bToABytes":2000}]}
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

cat >"$evidence/after-topology.json" <<'EOF'
{"businessEdge":{"bytesPerSecond":3000,"forwardPositive":true,"reversePositive":true}}
EOF
cat >"$evidence/after-history.raw.json" <<'EOF'
{"traffic":[{"aToBBytes":70000000,"bToABytes":2000}]}
EOF
if TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" \
  "$helper" assert-continuity before after >/dev/null 2>&1; then
  echo "exporter dogfood accepted a History catch-up spike" >&2
  exit 1
fi

touch "$evidence/private.raw.json" "$evidence/private.raw.log" "$evidence/safe.json"
TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" "$helper" purge-raw >/dev/null
test ! -e "$evidence/private.raw.json"
test ! -e "$evidence/private.raw.log"
test -e "$evidence/safe.json"

outside="$temporary/outside"
touch "$outside"
cat >"$runtime_file" <<EOF
TAILPATH_VERSION=edge-0123456789abcdef0123456789abcdef01234567
TAILPATH_EXPORTER_DOGFOOD_PREFIX=tailpath-exporter-dogfood
TAILPATH_EXPORTER_DOGFOOD_EVIDENCE=$evidence/../$(basename "$temporary")
TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE=$auth_file
EOF
if TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" "$helper" purge-raw >/dev/null 2>&1; then
  echo "exporter dogfood accepted traversal in runtime evidence" >&2
  exit 1
fi
test -e "$outside"

runtime_link=${runtime_file}.link
ln -s "$runtime_file" "$runtime_link"
if TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_link" "$helper" status >/dev/null 2>&1; then
  echo "exporter dogfood accepted a symbolic-link runtime state" >&2
  exit 1
fi
rm -f "$runtime_link"
