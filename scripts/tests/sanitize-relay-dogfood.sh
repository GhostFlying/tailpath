#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
sanitizer="$root/scripts/sanitize-relay-dogfood.sh"
temporary=$(mktemp -d /tmp/tailpath-relay-sanitizer.XXXXXX)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

cat >"$temporary/topology.json" <<'EOF'
{
  "edges": [{
    "id": "private-edge-id",
    "source": "private-source-id",
    "target": "private-target-id",
    "path": {
      "kind": "peer_relay",
      "peerRelayStableNodeId": "private-relay-id",
      "peerRelayVni": 7
    },
    "state": "active",
    "aToBBytesPerSecond": 1200,
    "bToABytesPerSecond": 0,
    "lastActive": "2026-08-26T12:00:00Z",
    "observations": [{
      "observerId": "private-observer-id",
      "clockSkewed": false,
      "relaySession": {
        "sessionId": "private-session-id",
        "sourceIdentityStatus": "resolved",
        "targetIdentityStatus": "anonymous"
      }
    }]
  }]
}
EOF

"$sanitizer" topology no-collector private-edge-id <"$temporary/topology.json" >"$temporary/sanitized.json"
jq -e '
  .scenario == "no-collector" and
  .path == "peer_relay" and
  .state == "active" and
  .directionalTraffic.forwardPositive == true and
  .directionalTraffic.reversePositive == false and
  .relay == {"present": true, "vniPresent": true} and
  .provenance.count == 1 and
  .provenance.relaySessionCount == 1 and
  .provenance.identityStatuses == ["anonymous", "resolved"] and
  .provenance.clockSkewed == false
' "$temporary/sanitized.json" >/dev/null

for private in private-edge-id private-source-id private-target-id private-relay-id private-observer-id private-session-id; do
  if grep -F "$private" "$temporary/sanitized.json" >/dev/null; then
    echo "sanitized topology leaked $private" >&2
    exit 1
  fi
done

cat >"$temporary/check.json" <<'EOF'
{
  "self": {"stableNodeId": "private-node-id", "hostname": "private-host"},
  "os": "linux",
  "peerCount": 12,
  "relayCapability": "enabled",
  "relayEnabled": true,
  "relaySessionCount": 2
}
EOF
"$sanitizer" collector-check <"$temporary/check.json" >"$temporary/check-sanitized.json"
jq -e '
  . == {
    "version": 1,
    "os": "linux",
    "peerCount": 12,
    "relayCapability": "enabled",
    "relayEnabled": true,
    "relaySessionCount": 2
  }
' "$temporary/check-sanitized.json" >/dev/null
if grep -E 'private-node-id|private-host|self' "$temporary/check-sanitized.json" >/dev/null; then
  echo "sanitized collector check leaked identity" >&2
  exit 1
fi

if "$sanitizer" topology missing private-missing <"$temporary/topology.json" >/dev/null 2>&1; then
  echo "sanitizer accepted a missing edge" >&2
  exit 1
fi
if "$sanitizer" topology 'unsafe scenario' private-edge-id <"$temporary/topology.json" >/dev/null 2>&1; then
  echo "sanitizer accepted an unsafe scenario" >&2
  exit 1
fi
