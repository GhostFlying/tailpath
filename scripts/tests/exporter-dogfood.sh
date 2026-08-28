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
TAILPATH_EXPORTER_DOGFOOD_PROJECT=tailpath-exporter-dogfood-fixture
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
{"to":"2026-08-28T12:00:00.250Z","traffic":[{"bucketStart":"2026-08-28T11:59:50.250Z","aToBBytes":1000,"bToABytes":1000}]}
EOF
cat >"$evidence/after-history.raw.json" <<'EOF'
{"to":"2026-08-28T12:00:20.750Z","traffic":[{"bucketStart":"2026-08-28T11:59:50.750Z","aToBBytes":70000000,"bToABytes":2000},{"bucketStart":"2026-08-28T12:00:10.750Z","aToBBytes":1000,"bToABytes":1000}]}
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
{"to":"2026-08-28T12:00:20.750Z","traffic":[{"bucketStart":"2026-08-28T12:00:10.750Z","aToBBytes":70000000,"bToABytes":2000}]}
EOF
if TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" \
  "$helper" assert-continuity before after >/dev/null 2>&1; then
  echo "exporter dogfood accepted a History catch-up spike" >&2
  exit 1
fi

cat >"$evidence/after-history.raw.json" <<'EOF'
{"to":"2026-08-28T12:00:20.750Z","traffic":[{"bucketStart":"2026-08-28T12:00:00.750Z","aToBBytes":70000000,"bToABytes":2000}]}
EOF
if TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" \
  "$helper" assert-continuity before after >/dev/null 2>&1; then
  echo "exporter dogfood accepted a boundary History catch-up spike" >&2
  exit 1
fi

touch "$evidence/private.raw.json" "$evidence/private.raw.log" "$evidence/safe.json"
TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" "$helper" purge-raw >/dev/null
test ! -e "$evidence/private.raw.json"
test ! -e "$evidence/private.raw.log"
test -e "$evidence/safe.json"

fake_bin=$temporary/bin
fake_log=$temporary/docker.log
mkdir "$fake_bin"
cat >"$fake_bin/docker" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$TAILPATH_FAKE_DOCKER_LOG"
if test "${TAILPATH_FAKE_PROJECT_DIRTY:-false}" = true && test "${1:-}" = ps; then
  echo contaminated-resource
fi
EOF
chmod 0755 "$fake_bin/docker"

chmod u+w "$auth_file"
printf 'fixture-key' >"$auth_file"
chmod 0444 "$auth_file"
: >"$fake_log"
if PATH="$fake_bin:$PATH" TAILPATH_FAKE_DOCKER_LOG="$fake_log" TAILPATH_FAKE_PROJECT_DIRTY=true \
  TAILPATH_VERSION=edge-0123456789abcdef0123456789abcdef01234567 \
  TAILPATH_EXPORTER_DOGFOOD_PROJECT=tailpath-exporter-dogfood-fixture \
  TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE="$auth_file" \
  TAILPATH_EXPORTER_DOGFOOD_EVIDENCE="$evidence" "$helper" up >/dev/null 2>&1; then
  echo "exporter dogfood accepted a contaminated Compose project" >&2
  exit 1
fi
test "$(grep -cF 'ps -aq --filter label=com.docker.compose.project=tailpath-exporter-dogfood-fixture' "$fake_log")" -eq 1
if grep -E 'compose .* (pull|up)( |$)' "$fake_log" >/dev/null; then
  echo "exporter dogfood mutated a contaminated Compose project" >&2
  exit 1
fi
: >"$fake_log"
PATH="$fake_bin:$PATH" TAILPATH_FAKE_DOCKER_LOG="$fake_log" \
  TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" "$helper" status >/dev/null
test "$(wc -l <"$fake_log")" -eq 2
test "$(grep -cF 'compose -p tailpath-exporter-dogfood-fixture' "$fake_log")" -eq 2
if PATH="$fake_bin:$PATH" TAILPATH_FAKE_DOCKER_LOG="$fake_log" \
  TAILPATH_EXPORTER_DOGFOOD_PROJECT=tailpath-exporter-dogfood-other \
  TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" "$helper" status >/dev/null 2>&1; then
  echo "exporter dogfood accepted a Compose project mismatch" >&2
  exit 1
fi

chmod 0755 "$evidence"
if PATH="$fake_bin:$PATH" TAILPATH_FAKE_DOCKER_LOG="$fake_log" \
  TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE="$runtime_file" "$helper" status >/dev/null 2>&1; then
  echo "exporter dogfood accepted evidence permission drift" >&2
  exit 1
fi
chmod 0700 "$evidence"

outside="$temporary/outside"
touch "$outside"
cat >"$runtime_file" <<EOF
TAILPATH_VERSION=edge-0123456789abcdef0123456789abcdef01234567
TAILPATH_EXPORTER_DOGFOOD_PROJECT=tailpath-exporter-dogfood-fixture
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
