#!/bin/sh
set -eu

usage() {
  echo "usage: sanitize-relay-dogfood.sh <topology SCENARIO EDGE_ID|collector-check>" >&2
}

mode=${1:-}
case "$mode" in
  topology)
    test "$#" -eq 3 || { usage; exit 2; }
    scenario=$2
    edge_id=$3
    case "$scenario" in
      ""|*[!A-Za-z0-9_-]*|?????????????????????????????????*)
        echo "scenario must contain 1-32 ASCII letters, digits, underscores, or hyphens" >&2
        exit 2
        ;;
    esac
    test -n "$edge_id" || { echo "edge ID is required" >&2; exit 2; }
    jq -e --arg scenario "$scenario" --arg edgeID "$edge_id" '
      [ .edges[]? | select(.id == $edgeID) ] as $matches
      | if ($matches | length) != 1 then
          error("selected edge must appear exactly once")
        else $matches[0] end
      | {
          version: 1,
          scenario: $scenario,
          generatedAt: .lastActive,
          path: .path.kind,
          state: .state,
          directionalTraffic: {
            forwardPositive: ((.aToBBytesPerSecond // 0) > 0),
            reversePositive: ((.bToABytesPerSecond // 0) > 0)
          },
          relay: {
            present: ((.path.peerRelayStableNodeId // "") != ""),
            vniPresent: (.path.peerRelayVni != null)
          },
          provenance: {
            count: ([.observations[]?] | length),
            relaySessionCount: ([.observations[]? | select(.relaySession != null)] | length),
            identityStatuses: ([
              .observations[]?.relaySession?
              | .sourceIdentityStatus, .targetIdentityStatus
              | select(. != null)
            ] | unique | sort),
            clockSkewed: any(.observations[]?; .clockSkewed == true)
          }
        }
    '
    ;;
  collector-check)
    test "$#" -eq 1 || { usage; exit 2; }
    jq -e '
      if (.os | type) != "string" or
         (.peerCount | type) != "number" or
         (.relayCapability | IN("enabled", "disabled", "unsupported", "transient")) != true or
         (.relayEnabled | type) != "boolean" or
         (.relaySessionCount | type) != "number"
      then error("invalid collector diagnostic")
      else {
        version: 1,
        os,
        peerCount,
        relayCapability,
        relayEnabled,
        relaySessionCount
      } end
    '
    ;;
  *)
    usage
    exit 2
    ;;
esac
