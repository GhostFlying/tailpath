#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
sanitizer="$root/scripts/sanitize-exporter-dogfood.sh"
temporary=$(mktemp -d /tmp/tailpath-exporter-sanitizer.XXXXXX)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

cat >"$temporary/topology.json" <<'EOF'
{
  "nodes": [
    {"id":"private-a-id","hostname":"private-runtime-a"},
    {"id":"private-b-id","hostname":"private-runtime-b"},
    {"id":"private-c-id","hostname":"private-runtime-c"},
    {"id":"private-reporter-id","hostname":"private-reporter"}
  ],
  "observers": [
    {"id":"private-a-id","online":true,"clockSkewed":false},
    {"id":"private-b-id","online":true,"clockSkewed":false},
    {"id":"private-c-id","online":false,"clockSkewed":true}
  ],
  "edges": [{
    "id":"private-edge-id","source":"private-a-id","target":"private-b-id",
    "systemTelemetry":false,"path":{"kind":"derp","derpRegion":"private-region"},
    "state":"active","aToBBytesPerSecond":1200,"bToABytesPerSecond":30,
    "observations":[{"observerId":"private-a-id"},{"observerId":"private-b-id"}]
  }]
}
EOF

"$sanitizer" topology derp private-runtime-a private-runtime-b private-runtime-c private-reporter \
  <"$temporary/topology.json" >"$temporary/topology-sanitized.json"
jq -e '
  . == {
    "version":1,"scenario":"derp","observerCount":3,
    "runtimes":{"known":3,"reporting":2,"stale":1},
    "reporter":{"presentAsNode":true,"presentAsObserver":false},
    "businessEdge":{"path":"derp","state":"active","forwardPositive":true,
      "reversePositive":true,"bytesPerSecond":1230,"provenanceCount":2,"systemTelemetry":false},
    "clockWarnings":1
  }
' "$temporary/topology-sanitized.json" >/dev/null

cat >"$temporary/history.json" <<'EOF'
{
  "edgeId":"private-edge-id","source":{"id":"private-a-id"},"target":{"id":"private-b-id"},
  "traffic":[{"bucketStart":"2026-08-28T00:00:00Z","aToBBytes":100,"bToABytes":20}],
  "pathAnchor":{"path":{"kind":"direct"}},
  "pathEvents":[{"path":{"kind":"derp"}},{"path":{"kind":"direct"}}],
  "trafficTruncated":false,"pathEventsTruncated":false
}
EOF
"$sanitizer" history restored <"$temporary/history.json" >"$temporary/history-sanitized.json"
jq -e '
  .scenario == "restored" and .trafficPoints == 1 and
  .directionalTraffic == {"forwardPositive":true,"reversePositive":true} and
  .pathKinds == ["derp","direct"] and .pathEvents == 2 and
  .trafficTruncated == false and .pathEventsTruncated == false
' "$temporary/history-sanitized.json" >/dev/null

for private in private-a-id private-b-id private-c-id private-reporter-id private-edge-id private-region private-runtime; do
  if grep -F "$private" "$temporary/topology-sanitized.json" "$temporary/history-sanitized.json" >/dev/null; then
    echo "sanitized evidence leaked $private" >&2
    exit 1
  fi
done

if printf '%s\n' '{"nodes":[],"observers":[],"edges":[]}' \
  | "$sanitizer" topology missing private-runtime-a private-runtime-b private-runtime-c private-reporter \
    >/dev/null 2>&1; then
  echo "sanitizer accepted missing runtime identities" >&2
  exit 1
fi
if "$sanitizer" history 'unsafe scenario' <"$temporary/history.json" >/dev/null 2>&1; then
  echo "sanitizer accepted an unsafe scenario" >&2
  exit 1
fi
