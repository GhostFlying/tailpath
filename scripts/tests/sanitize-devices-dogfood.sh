#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
sanitizer="$root/scripts/sanitize-devices-dogfood.sh"
temporary=$(mktemp -d /tmp/tailpath-devices-sanitizer.XXXXXX)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
chmod 0700 "$temporary"

cat >"$temporary/topology.json" <<'EOF'
{
  "nodes":[{"id":"private-runtime-id"}],
  "edges":[{"source":"private-runtime-id","target":"private-peer-id"}]
}
EOF
cat >"$temporary/healthy.json" <<'EOF'
{
  "sync":{"status":"healthy","lastSuccessAt":"2026-08-31T00:00:00.123456789Z","invalidAddressCount":1},
  "devices":[
    {
      "id":"private-runtime-id","stableNodeId":"private-stable-a",
      "dnsName":"private-a.example.ts.net","hostname":"private-a","platform":"linux",
      "tailscaleIps":["100.64.0.1"],"tags":["tag:private"],"connectedToControl":true,
      "collectedAt":"2026-08-31T00:00:00.123456789Z",
      "runtime":{"observable":true,"online":true,"lastEvidenceAt":"2026-08-31T00:00:00Z"},
      "conflicts":[{"field":"hostname","directoryValues":["private-a"],"runtimeValues":["private-old"]}]
    },
    {
      "id":"private-directory-only-id","stableNodeId":"private-stable-b",
      "dnsName":"private-b.example.ts.net","hostname":"private-b","platform":"ios",
      "tailscaleIps":["fd7a:115c:a1e0::1"],"tags":[],"connectedToControl":false,
      "collectedAt":"2026-08-31T00:00:00.123456789Z",
      "runtime":null,"conflicts":[]
    }
  ]
}
EOF

"$sanitizer" snapshot initial "$temporary/topology.json" \
  <"$temporary/healthy.json" >"$temporary/snapshot-safe.json"
jq -e '
  .version == 1 and .scenario == "initial" and
  .sync == {"status":"healthy","errorCode":null,"invalidAddressCount":1} and
  .directory.deviceCount == 2 and .directory.controlConnectedCount == 1 and
  .directory.runtimeEvidenceCount == 1 and .directory.runtimeObservableCount == 1 and
  .directory.runtimeOnlineCount == 1 and .directory.directoryOnlyCount == 1 and
  .directory.conflictDeviceCount == 1 and .directory.conflictCount == 1 and
  (.directory.identitySetSha256 | test("^[0-9a-f]{64}$")) and
  (.directory.contentSha256 | test("^[0-9a-f]{64}$")) and
  (.directory.canonicalMappingSha256 | test("^[0-9a-f]{64}$")) and
  .liveIsolation == {"topologyNodeCount":1,"topologyEdgeCount":1,
    "directoryOnlyAbsentFromTopology":true,"directoryOnlyAbsentFromEdges":true}
' "$temporary/snapshot-safe.json" >/dev/null

jq '.sync.status = "stale" | .sync.errorCode = "unauthorized" |
  .devices[0].runtime.lastEvidenceAt = "2026-08-31T00:01:00Z" |
  .devices[0].runtime.online = false |
  .devices[0].conflicts[0].runtimeCollectedAt = "2026-08-31T00:01:00Z"' \
  "$temporary/healthy.json" >"$temporary/stale.json"
"$sanitizer" compare stale "$temporary/healthy.json" "$temporary/stale.json" \
  >"$temporary/compare-safe.json"
jq -e '
  .version == 1 and .scenario == "stale" and
  .before.status == "healthy" and .after.status == "stale" and
  .after.errorCode == "unauthorized" and .before.deviceCount == 2 and
  .after.deviceCount == 2 and .lastGoodPreserved == true and
  .successAdvancedSeconds == 0 and
  .canonicalMappingStable == true and
  .before.contentSha256 == .after.contentSha256
' "$temporary/compare-safe.json" >/dev/null

jq '.devices[0].hostname = "private-directory-change"' \
  "$temporary/stale.json" >"$temporary/directory-changed.json"
"$sanitizer" compare directory-change "$temporary/healthy.json" \
  "$temporary/directory-changed.json" >"$temporary/directory-change-safe.json"
jq -e '
  .lastGoodPreserved == false and
  .before.identitySetSha256 == .after.identitySetSha256 and
  .before.contentSha256 != .after.contentSha256
' "$temporary/directory-change-safe.json" >/dev/null

cat >"$temporary/renewed.json" <<'EOF'
{
  "sync":{"status":"healthy","lastSuccessAt":"2026-08-31T01:06:00Z","invalidAddressCount":0},
  "devices":[
    {"id":"another-private-id-a","stableNodeId":"private-stable-a","connectedToControl":false,"collectedAt":"2026-08-31T01:06:00Z"},
    {"id":"another-private-id-b","stableNodeId":"private-stable-b","connectedToControl":false,"collectedAt":"2026-08-31T01:06:00Z"}
  ]
}
EOF
"$sanitizer" compare renewal "$temporary/healthy.json" "$temporary/renewed.json" \
  >"$temporary/renewal-safe.json"
jq -e '
  .after.status == "healthy" and .successAdvancedSeconds == 3960 and
  .before.identitySetSha256 == .after.identitySetSha256 and
  (.lastGoodPreserved | not)
' \
  "$temporary/renewal-safe.json" >/dev/null

for private in private-runtime private-peer private-stable private-canonical private-a private-b tag:private example.ts.net 100.64 fd7a; do
  if grep -F "$private" "$temporary/snapshot-safe.json" "$temporary/compare-safe.json" \
    "$temporary/directory-change-safe.json" "$temporary/renewal-safe.json" >/dev/null; then
    echo "sanitized Devices evidence leaked $private" >&2
    exit 1
  fi
done

if "$sanitizer" snapshot 'unsafe scenario' "$temporary/topology.json" <"$temporary/healthy.json" >/dev/null 2>&1; then
  echo "sanitizer accepted an unsafe scenario" >&2
  exit 1
fi
cat >"$temporary/duplicate.json" <<'EOF'
{"sync":{"status":"healthy"},"devices":[{"stableNodeId":"same"},{"stableNodeId":"same"}]}
EOF
if "$sanitizer" snapshot duplicate "$temporary/topology.json" <"$temporary/duplicate.json" >/dev/null 2>&1; then
  echo "sanitizer accepted duplicate StableNodeIDs" >&2
  exit 1
fi
cat >"$temporary/raw-error.json" <<'EOF'
{"sync":{"status":"stale","errorCode":"private-upstream-body"},"devices":[]}
EOF
if "$sanitizer" compare raw-error "$temporary/healthy.json" "$temporary/raw-error.json" >/dev/null 2>&1; then
  echo "sanitizer accepted an unbounded error value" >&2
  exit 1
fi
