#!/bin/sh
set -eu

usage() {
  echo "usage: sanitize-devices-dogfood.sh snapshot SCENARIO TOPOLOGY_JSON | compare SCENARIO BEFORE_JSON AFTER_JSON" >&2
}

validate_scenario() {
  case "$1" in
    ""|*[!A-Za-z0-9_-]*|?????????????????????????????????*)
      echo "scenario must contain 1-32 ASCII letters, digits, underscores, or hyphens" >&2
      exit 2
      ;;
  esac
}

validate_private_file() {
  test -f "$1" && test ! -L "$1" || {
    echo "private capture must be a regular, non-symlink file" >&2
    exit 2
  }
}

identity_hash() {
  canonical=$(jq -cer '
    if (.devices | type) != "array" then error("invalid device directory") else . end
    | [.devices[].stableNodeId] as $identities
    | if any($identities[]; type != "string" or length == 0) then
        error("invalid stable node identity")
      elif ($identities | unique | length) != ($identities | length) then
        error("duplicate stable node identity")
      else $identities | sort end
  ' "$1") || return
  printf '%s' "$canonical" | sha256sum | cut -d ' ' -f 1
}

directory_hash() {
  canonical=$(jq -cer '
    if (.devices | type) != "array" then error("invalid device directory") else . end
    | [.devices[].stableNodeId] as $identities
    | if any($identities[]; type != "string" or length == 0) or
        (($identities | unique | length) != ($identities | length)) then
        error("invalid stable node identities")
      else .devices | sort_by(.stableNodeId) end
  ' "$1") || return
  printf '%s' "$canonical" | sha256sum | cut -d ' ' -f 1
}

mode=${1:-}
case "$mode" in
  snapshot)
    test "$#" -eq 3 || { usage; exit 2; }
    scenario=$2
    topology_file=$3
    validate_scenario "$scenario"
    validate_private_file "$topology_file"
    temporary=$(mktemp /tmp/tailpath-devices-snapshot.XXXXXX)
    trap 'rm -f "$temporary"' EXIT HUP INT TERM
    umask 077
    cat >"$temporary"
    hash=$(identity_hash "$temporary")
    content_hash=$(directory_hash "$temporary")
    jq -e \
      --arg scenario "$scenario" \
      --arg identityHash "$hash" \
      --arg contentHash "$content_hash" \
      --slurpfile topology "$topology_file" '
      . as $directory
      | if ($topology | length) != 1 or (($topology[0].nodes | type) != "array") or
          (($topology[0].edges | type) != "array") then
          error("invalid topology capture")
        elif (.sync.status | IN("disabled", "syncing", "healthy", "stale") | not) then
          error("invalid directory sync status")
        elif ((.sync.errorCode // null) as $code |
          ($code != null and ($code | IN("unauthorized", "forbidden", "rate-limited", "unavailable", "timeout", "invalid-response") | not))) then
          error("invalid directory error code")
        elif ((.sync.invalidAddressCount // 0) | type) != "number" or
          ((.sync.invalidAddressCount // 0) < 0) then
          error("invalid address count")
        else . end
      | [.devices[] | select(.runtime == null)] as $directoryOnly
      | [$directoryOnly[].id] as $directoryOnlyIDs
      | {
          version: 1,
          scenario: $scenario,
          sync: {
            status: .sync.status,
            errorCode: (.sync.errorCode // null),
            invalidAddressCount: (.sync.invalidAddressCount // 0)
          },
          directory: {
            deviceCount: (.devices | length),
            identitySetSha256: $identityHash,
            contentSha256: $contentHash,
            controlConnectedCount: ([.devices[] | select(.connectedToControl == true)] | length),
            runtimeEvidenceCount: ([.devices[] | select(.runtime != null)] | length),
            runtimeObservableCount: ([.devices[] | select(.runtime.observable == true)] | length),
            runtimeOnlineCount: ([.devices[] | select(.runtime.online == true)] | length),
            directoryOnlyCount: ($directoryOnly | length),
            conflictDeviceCount: ([.devices[] | select((.conflicts | length) > 0)] | length),
            conflictCount: ([.devices[].conflicts[]?] | length)
          },
          liveIsolation: {
            topologyNodeCount: ($topology[0].nodes | length),
            topologyEdgeCount: ($topology[0].edges | length),
            directoryOnlyAbsentFromTopology:
              (all($directoryOnlyIDs[]; . as $id | all($topology[0].nodes[]; .id != $id))),
            directoryOnlyAbsentFromEdges:
              (all($directoryOnlyIDs[]; . as $id |
                all($topology[0].edges[]; .source != $id and .target != $id)))
          }
        }
    ' "$temporary"
    ;;
  compare)
    test "$#" -eq 4 || { usage; exit 2; }
    scenario=$2
    before_file=$3
    after_file=$4
    validate_scenario "$scenario"
    validate_private_file "$before_file"
    validate_private_file "$after_file"
    before_hash=$(identity_hash "$before_file")
    after_hash=$(identity_hash "$after_file")
    before_content_hash=$(directory_hash "$before_file")
    after_content_hash=$(directory_hash "$after_file")
    jq -en \
      --arg scenario "$scenario" \
      --arg beforeHash "$before_hash" \
      --arg afterHash "$after_hash" \
      --arg beforeContentHash "$before_content_hash" \
      --arg afterContentHash "$after_content_hash" \
      --slurpfile before "$before_file" \
      --slurpfile after "$after_file" '
      def syncTime($value):
        if $value == null then null
        else ($value | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601) end;
      def validStatus($value): $value | IN("disabled", "syncing", "healthy", "stale");
      def validError($value):
        $value == null or ($value | IN("unauthorized", "forbidden", "rate-limited", "unavailable", "timeout", "invalid-response"));
      if ($before | length) != 1 or ($after | length) != 1 or
          (($before[0].devices | type) != "array") or (($after[0].devices | type) != "array") then
        error("invalid device directory comparison")
      elif (validStatus($before[0].sync.status) | not) or
          (validStatus($after[0].sync.status) | not) or
          (validError($before[0].sync.errorCode // null) | not) or
          (validError($after[0].sync.errorCode // null) | not) then
        error("invalid directory sync state")
      else
        (syncTime($before[0].sync.lastSuccessAt)) as $beforeSuccess
        | (syncTime($after[0].sync.lastSuccessAt)) as $afterSuccess
        | {
            version: 1,
            scenario: $scenario,
            before: {
              status: $before[0].sync.status,
              errorCode: ($before[0].sync.errorCode // null),
              deviceCount: ($before[0].devices | length),
              identitySetSha256: $beforeHash,
              contentSha256: $beforeContentHash
            },
            after: {
              status: $after[0].sync.status,
              errorCode: ($after[0].sync.errorCode // null),
              deviceCount: ($after[0].devices | length),
              identitySetSha256: $afterHash,
              contentSha256: $afterContentHash
            },
            lastGoodPreserved:
              (($before[0].devices | length) == ($after[0].devices | length) and
                $beforeHash == $afterHash and $beforeContentHash == $afterContentHash),
            successAdvancedSeconds:
              (if $beforeSuccess == null or $afterSuccess == null then null
               else ($afterSuccess - $beforeSuccess) end)
          }
      end
    '
    ;;
  *)
    usage
    exit 2
    ;;
esac
