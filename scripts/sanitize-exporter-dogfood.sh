#!/bin/sh
set -eu

usage() {
  echo "usage: sanitize-exporter-dogfood.sh topology SCENARIO HOST_A HOST_B HOST_C REPORTER_HOST | history SCENARIO" >&2
}

mode=${1:-}
case "$mode" in
  topology)
    test "$#" -eq 6 || { usage; exit 2; }
    scenario=$2
    host_a=$3
    host_b=$4
    host_c=$5
    reporter_host=$6
    case "$scenario" in
      ""|*[!A-Za-z0-9_-]*|?????????????????????????????????*)
        echo "scenario must contain 1-32 ASCII letters, digits, underscores, or hyphens" >&2
        exit 2
        ;;
    esac
    for value in "$host_a" "$host_b" "$host_c" "$reporter_host"; do
      test -n "$value" || { echo "all private host references are required" >&2; exit 2; }
    done
    jq -e \
      --arg scenario "$scenario" \
      --arg hostA "$host_a" \
      --arg hostB "$host_b" \
      --arg hostC "$host_c" \
      --arg reporterHost "$reporter_host" '
      . as $top
      | def node($host): [$top.nodes[]? | select(.hostname == $host)];
      node($hostA) as $a
      | node($hostB) as $b
      | node($hostC) as $c
      | node($reporterHost) as $reporter
      | if ($a|length) != 1 or ($b|length) != 1 or ($c|length) != 1 then
          error("each application runtime must resolve exactly once")
        elif ($reporter|length) > 1 then
          error("reporter identity is ambiguous")
        else . end
      | [$a[0].id, $b[0].id, $c[0].id] as $runtimeIDs
      | [$a[0].id, $b[0].id] as $businessIDs
      | [ $top.edges[]? |
          select(([.source, .target] | sort) == ($businessIDs | sort))
        ] as $business
      | (if ($business|length) != 1 then
          error("runtime A-B business edge must resolve exactly once")
        else $business[0] end) as $edge
      | {
          version: 1,
          scenario: $scenario,
          observerCount: ([$top.observers[]?] | length),
          runtimes: {
            known: ($runtimeIDs | length),
            reporting: ([$top.observers[]? | select(.id as $id | $runtimeIDs | index($id)) | select(.online)] | length),
            stale: ([$top.observers[]? | select(.id as $id | $runtimeIDs | index($id)) | select(.online | not)] | length)
          },
          reporter: {
            presentAsNode: (($reporter | length) == 1),
            presentAsObserver: (if ($reporter | length) == 0 then false else any($top.observers[]?; .id == $reporter[0].id) end)
          },
          businessEdge: {
            path: $edge.path.kind,
            state: $edge.state,
            forwardPositive: (($edge.aToBBytesPerSecond // 0) > 0),
            reversePositive: (($edge.bToABytesPerSecond // 0) > 0),
            bytesPerSecond: (($edge.aToBBytesPerSecond // 0) + ($edge.bToABytesPerSecond // 0)),
            provenanceCount: ([$edge.observations[]?] | length),
            systemTelemetry: $edge.systemTelemetry
          },
          clockWarnings: ([$top.observers[]? | select(.clockSkewed)] | length)
        }
    '
    ;;
  history)
    test "$#" -eq 2 || { usage; exit 2; }
    scenario=$2
    case "$scenario" in
      ""|*[!A-Za-z0-9_-]*|?????????????????????????????????*)
        echo "scenario must contain 1-32 ASCII letters, digits, underscores, or hyphens" >&2
        exit 2
        ;;
    esac
    jq -e --arg scenario "$scenario" '
      if (.traffic | type) != "array" or (.pathEvents | type) != "array" then
        error("invalid History detail")
      else {
        version: 1,
        scenario: $scenario,
        trafficPoints: (.traffic | length),
        directionalTraffic: {
          forwardPositive: (any(.traffic[]?; (.aToBBytes // 0) > 0)),
          reversePositive: (any(.traffic[]?; (.bToABytes // 0) > 0))
        },
        pathKinds: ([.pathAnchor?.path.kind, .pathEvents[]?.path.kind] | map(select(. != null)) | unique | sort),
        pathEvents: (.pathEvents | length),
        trafficTruncated: .trafficTruncated,
        pathEventsTruncated: .pathEventsTruncated
      } end
    '
    ;;
  *)
    usage
    exit 2
    ;;
esac
