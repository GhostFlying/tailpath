#!/bin/sh
set -eu

repository=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
compose_file="$repository/compose.exporter-dogfood.yaml"
sanitizer="$repository/scripts/sanitize-exporter-dogfood.sh"
if test "${TAILPATH_EXPORTER_DOGFOOD_PROJECT+x}" = x; then
  project_explicit=true
  project=$TAILPATH_EXPORTER_DOGFOOD_PROJECT
else
  project_explicit=false
  project=tailpath-exporter-dogfood
fi
auth_file=${TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE:-/tmp/tailpath-exporter-dogfood-secret/authkey}
runtime_file=${TAILPATH_EXPORTER_DOGFOOD_RUNTIME_FILE:-/tmp/tailpath-exporter-dogfood-runtime.env}
udp_chain=TAILPATH-EXPORTER-DOGFOOD

fail() {
  echo "tailpath exporter dogfood: $*" >&2
  exit 1
}

validate_project() {
  project_value=$1
  test -n "$project_value" || fail "project is required"
  test "${#project_value}" -le 63 || fail "project is too long"
  case "$project_value" in
    tailpath-exporter-dogfood|tailpath-exporter-dogfood-*) ;;
    *) fail "project must use the tailpath-exporter-dogfood prefix" ;;
  esac
  test "$project_value" != tailpath-exporter-dogfood- || fail "project suffix is required after the separator"
  case "$project_value" in *[!a-z0-9_-]*) fail "project contains unsafe characters" ;; esac
}

validate_project "$project"

compose() {
  docker compose -p "$project" -f "$compose_file" "$@"
}

require_clean_project() {
  containers=$(docker ps -aq --filter "label=com.docker.compose.project=$project")
  volumes=$(docker volume ls -q --filter "label=com.docker.compose.project=$project")
  networks=$(docker network ls -q --filter "label=com.docker.compose.project=$project")
  test -z "$containers$volumes$networks" \
    || fail "project already has containers, volumes, or networks; run down and inspect evidence before a new qualification"
}

zero_key() {
  validate_secret_file "$auth_file"
  test ! -f "$auth_file" || chmod u+w "$auth_file"
  test ! -f "$auth_file" || : >"$auth_file"
  test ! -f "$auth_file" || chmod 0444 "$auth_file"
}

validate_tag() {
  tag=$1
  sha=${tag#edge-}
  test "$tag" != "$sha" || fail "TAILPATH_VERSION must be edge-<full-main-sha>"
  test "${#sha}" -eq 40 || fail "candidate SHA must contain 40 lowercase hex characters"
  case "$sha" in *[!0-9a-f]*) fail "candidate SHA must contain lowercase hex only" ;; esac
}

validate_prefix() {
  value=$1
  test -n "$value" || fail "hostname prefix is required"
  test "${#value}" -le 32 || fail "hostname prefix is too long"
  case "$value" in *[!a-z0-9-]*) fail "hostname prefix must use lowercase letters, digits, and hyphens" ;; esac
}

validate_private_path() {
  value=$1
  prefix=$2
  label=$3
  case "$value" in
    "$prefix"|"$prefix".*) ;;
    *) fail "$label must stay under its dedicated /tmp prefix" ;;
  esac
  case "$value" in *[!A-Za-z0-9_./-]*) fail "$label contains unsafe characters" ;; esac
  suffix=${value#"$prefix"}
  case "$suffix" in *..*|*/*) fail "$label must not contain traversal components" ;; esac
}

validate_secret_file() {
  secret_file=$1
  secret_parent=${secret_file%/*}
  test "${secret_file##*/}" = authkey || fail "auth key file must be named authkey"
  validate_private_path "$secret_parent" /tmp/tailpath-exporter-dogfood-secret "auth key directory"
  test ! -L "$secret_parent" || fail "auth key directory must not be a symbolic link"
  test -d "$secret_parent" || fail "auth key directory is missing"
  directory_permissions=$(stat -c '%a' "$secret_parent" 2>/dev/null || stat -f '%Lp' "$secret_parent")
  test "$directory_permissions" = 700 || fail "auth key directory mode must be 0700"
  test ! -L "$secret_file" || fail "auth key file must not be a symbolic link"
  test -f "$secret_file" || fail "auth key file is missing"
  file_permissions=$(stat -c '%a' "$secret_file" 2>/dev/null || stat -f '%Lp' "$secret_file")
  test "$file_permissions" = 444 || fail "auth key file mode must be 0444 inside its mode-0700 directory"
}

validate_evidence_dir() {
  evidence_dir=$1
  validate_private_path "$evidence_dir" /tmp/tailpath-exporter-dogfood-evidence "evidence directory"
  test ! -L "$evidence_dir" || fail "evidence directory must not be a symbolic link"
  test -d "$evidence_dir" || fail "evidence directory is missing"
  evidence_permissions=$(stat -c '%a' "$evidence_dir" 2>/dev/null || stat -f '%Lp' "$evidence_dir")
  test "$evidence_permissions" = 700 || fail "evidence directory mode must be 0700"
}

write_runtime() {
  validate_private_path "$runtime_file" /tmp/tailpath-exporter-dogfood-runtime "runtime state file"
  test ! -L "$runtime_file" || fail "runtime state file must not be a symbolic link"
  temporary=$(mktemp "${runtime_file}.XXXXXX")
  chmod 0600 "$temporary"
  {
    echo "TAILPATH_VERSION=$TAILPATH_VERSION"
    echo "TAILPATH_EXPORTER_DOGFOOD_PROJECT=$project"
    echo "TAILPATH_EXPORTER_DOGFOOD_PREFIX=$TAILPATH_EXPORTER_DOGFOOD_PREFIX"
    echo "TAILPATH_EXPORTER_DOGFOOD_EVIDENCE=$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE"
    echo "TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE=$auth_file"
  } >"$temporary"
  mv "$temporary" "$runtime_file"
}

runtime_value() {
  key=$1
  awk -F= -v key="$key" '
    $1 == key { count++; sub(/^[^=]*=/, ""); value=$0 }
    END { if (count != 1) exit 1; print value }
  ' "$runtime_file"
}

load_runtime() {
  validate_private_path "$runtime_file" /tmp/tailpath-exporter-dogfood-runtime "runtime state file"
  test ! -L "$runtime_file" || fail "runtime state file must not be a symbolic link"
  test -f "$runtime_file" || fail "run scripts/exporter-dogfood.sh up first"
  permissions=$(stat -c '%a' "$runtime_file" 2>/dev/null || stat -f '%Lp' "$runtime_file")
  test "$permissions" = 600 || fail "runtime state file mode must be 0600"
  TAILPATH_VERSION=$(runtime_value TAILPATH_VERSION) || fail "runtime state is missing TAILPATH_VERSION"
  runtime_project=$(runtime_value TAILPATH_EXPORTER_DOGFOOD_PROJECT) \
    || fail "runtime state is missing Compose project"
  validate_project "$runtime_project"
  if test "$project_explicit" = true && test "$project" != "$runtime_project"; then
    fail "explicit Compose project does not match runtime state"
  fi
  project=$runtime_project
  TAILPATH_EXPORTER_DOGFOOD_PREFIX=$(runtime_value TAILPATH_EXPORTER_DOGFOOD_PREFIX) \
    || fail "runtime state is missing hostname prefix"
  TAILPATH_EXPORTER_DOGFOOD_EVIDENCE=$(runtime_value TAILPATH_EXPORTER_DOGFOOD_EVIDENCE) \
    || fail "runtime state is missing evidence directory"
  TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE=$(runtime_value TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE) \
    || fail "runtime state is missing auth key file"
  validate_tag "$TAILPATH_VERSION"
  validate_prefix "$TAILPATH_EXPORTER_DOGFOOD_PREFIX"
  validate_evidence_dir "$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE"
  validate_secret_file "$TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE"
  auth_file=$TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE
  export TAILPATH_VERSION TAILPATH_EXPORTER_DOGFOOD_PREFIX
  export TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE
}

wait_healthy() {
  service=$1
  attempt=0
  while :; do
    health=$(compose ps "$service" --format json 2>/dev/null \
      | jq -rs '[.[] | if type == "array" then .[] else . end][0].Health // empty' 2>/dev/null || true)
    test "$health" != healthy || return 0
    attempt=$((attempt + 1))
    if test "$attempt" -ge 90; then
      compose logs --no-color "$service" >&2 || true
      fail "$service did not become healthy"
    fi
    sleep 2
  done
}

wait_exporter() {
  attempt=0
  while ! compose logs --no-color exporter 2>/dev/null | grep -F "tsnet exporter example started" >/dev/null; do
    attempt=$((attempt + 1))
    if test "$attempt" -ge 90; then
      compose logs --no-color exporter >&2 || true
      fail "exporter did not start"
    fi
    sleep 2
  done
}

exporter_log_count() {
  pattern=$1
  compose logs --no-color exporter 2>/dev/null | grep -cF "$pattern" || true
}

wait_exporter_log_increment() {
  pattern=$1
  previous=$2
  attempt=0
  while test "$(exporter_log_count "$pattern")" -le "$previous"; do
    attempt=$((attempt + 1))
    if test "$attempt" -ge 90; then
      compose logs --no-color exporter >&2 || true
      fail "exporter did not log $pattern"
    fi
    sleep 2
  done
}

server_ip() {
  hostname="${TAILPATH_EXPORTER_DOGFOOD_PREFIX}-server"
  compose exec -T inspector-tailscale tailscale --socket=/var/run/tailscale/tailscaled.sock status --json \
    | jq -er --arg hostname "$hostname" '
        [.Peer[]? | select(.HostName == $hostname) | .TailscaleIPs[0]][0] // empty
      '
}

api() {
  path=${1:-/api/v1/topology}
  ip=$(server_ip) || fail "inspector cannot resolve the Tailpath server"
  compose exec -T inspector wget -q -O - "http://$ip:8080$path"
}

runtime_hosts() {
  printf '%s\n' \
    "${TAILPATH_EXPORTER_DOGFOOD_PREFIX}-runtime-a" \
    "${TAILPATH_EXPORTER_DOGFOOD_PREFIX}-runtime-b" \
    "${TAILPATH_EXPORTER_DOGFOOD_PREFIX}-runtime-c" \
    "${TAILPATH_EXPORTER_DOGFOOD_PREFIX}-reporter"
}

wait_runtime_counts() {
  want_online=$1
  want_stale=$2
  attempt=0
  hosts=$(runtime_hosts)
  host_a=$(printf '%s\n' "$hosts" | sed -n '1p')
  host_b=$(printf '%s\n' "$hosts" | sed -n '2p')
  host_c=$(printf '%s\n' "$hosts" | sed -n '3p')
  reporter=$(printf '%s\n' "$hosts" | sed -n '4p')
  while :; do
    if topology=$(api /api/v1/topology 2>/dev/null) \
      && summary=$(printf '%s\n' "$topology" | "$sanitizer" topology wait "$host_a" "$host_b" "$host_c" "$reporter" 2>/dev/null) \
      && printf '%s\n' "$summary" | jq -e \
        --argjson online "$want_online" --argjson stale "$want_stale" '
          .runtimes.reporting == $online and .runtimes.stale == $stale and
          .observerCount == 3 and .reporter.presentAsObserver == false
        ' >/dev/null; then
      return 0
    fi
    attempt=$((attempt + 1))
    test "$attempt" -lt 90 || fail "runtime counts did not reach $want_online reporting / $want_stale stale"
    sleep 2
  done
}

capture() {
  scenario=${1:-}
  case "$scenario" in
    ""|*[!A-Za-z0-9_-]*|?????????????????????????????????*) fail "scenario must be 1-32 safe ASCII characters" ;;
  esac
  hosts=$(runtime_hosts)
  host_a=$(printf '%s\n' "$hosts" | sed -n '1p')
  host_b=$(printf '%s\n' "$hosts" | sed -n '2p')
  host_c=$(printf '%s\n' "$hosts" | sed -n '3p')
  reporter=$(printf '%s\n' "$hosts" | sed -n '4p')
  topology_raw="$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE/$scenario-topology.raw.json"
  topology_safe="$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE/$scenario-topology.json"
  api /api/v1/topology >"$topology_raw"
  "$sanitizer" topology "$scenario" "$host_a" "$host_b" "$host_c" "$reporter" \
    <"$topology_raw" >"$topology_safe"
  edge_id=$(jq -er --arg hostA "$host_a" --arg hostB "$host_b" '
    [.nodes[] | select(.hostname == $hostA)][0].id as $a
    | [.nodes[] | select(.hostname == $hostB)][0].id as $b
    | [.edges[] | select(([.source,.target]|sort) == ([$a,$b]|sort))][0].id
  ' "$topology_raw")
  case "$edge_id" in ""|*[!A-Za-z0-9_-]*) fail "business edge ID is unsafe" ;; esac
  history_raw="$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE/$scenario-history.raw.json"
  history_safe="$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE/$scenario-history.json"
  api "/api/v1/history/edges/$edge_id?window=15m" >"$history_raw"
  "$sanitizer" history "$scenario" <"$history_raw" >"$history_safe"
  jq -s '{topology:.[0],history:.[1]}' "$topology_safe" "$history_safe"
}

wait_path() {
  expected=${1:-}
  case "$expected" in direct|derp) ;; *) fail "path must be direct or derp" ;; esac
  attempt=0
  while :; do
    if capture "wait-$expected" | jq -e --arg expected "$expected" \
      '.topology.businessEdge.path == $expected and .topology.businessEdge.state == "active" and
       .topology.businessEdge.forwardPositive and .topology.businessEdge.reversePositive' >/dev/null; then
      return 0
    fi
    attempt=$((attempt + 1))
    test "$attempt" -lt 90 || fail "business edge did not become active $expected"
    sleep 2
  done
}

udp() {
  action=${1:-}
  case "$action" in derp|restore|status) ;; *) fail "udp action must be derp, restore, or status" ;; esac
  compose exec -T udp-helper sh -s -- "$action" "$udp_chain" <<'EOF'
set -eu
action=$1
chain=$2
for firewall in iptables ip6tables; do
  command -v "$firewall" >/dev/null 2>&1 || continue
  "$firewall" -S OUTPUT >/dev/null 2>&1 || continue
  case "$action" in
    derp)
      "$firewall" -n -L "$chain" >/dev/null 2>&1 || "$firewall" -N "$chain"
      "$firewall" -F "$chain"
      "$firewall" -A "$chain" -p udp --dport 53 -j RETURN
      "$firewall" -A "$chain" -p udp -j REJECT
      "$firewall" -C OUTPUT -j "$chain" >/dev/null 2>&1 || "$firewall" -I OUTPUT 1 -j "$chain"
      ;;
    restore)
      while "$firewall" -C OUTPUT -j "$chain" >/dev/null 2>&1; do
        "$firewall" -D OUTPUT -j "$chain"
      done
      if "$firewall" -n -L "$chain" >/dev/null 2>&1; then
        "$firewall" -F "$chain"
        "$firewall" -X "$chain"
      fi
      ;;
    status)
      if "$firewall" -C OUTPUT -j "$chain" >/dev/null 2>&1; then
        "$firewall" -S "$chain"
      else
        echo "$firewall open"
      fi
      ;;
  esac
done
EOF
}

up() {
  command -v docker >/dev/null 2>&1 || fail "docker is required"
  command -v jq >/dev/null 2>&1 || fail "jq is required"
  TAILPATH_VERSION=${TAILPATH_VERSION:-}
  validate_tag "$TAILPATH_VERSION"
  TAILPATH_EXPORTER_DOGFOOD_PREFIX=${TAILPATH_EXPORTER_DOGFOOD_PREFIX:-tailpath-exporter-dogfood}
  validate_prefix "$TAILPATH_EXPORTER_DOGFOOD_PREFIX"
  validate_secret_file "$auth_file"
  test -s "$auth_file" || fail "a non-empty reusable ephemeral key file is required"
  TAILPATH_EXPORTER_DOGFOOD_EVIDENCE=${TAILPATH_EXPORTER_DOGFOOD_EVIDENCE:-$(mktemp -d /tmp/tailpath-exporter-dogfood-evidence.XXXXXX)}
  validate_evidence_dir "$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE"
  require_clean_project
  export TAILPATH_VERSION TAILPATH_EXPORTER_DOGFOOD_PREFIX TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE
  export TAILPATH_EXPORTER_DOGFOOD_LIFECYCLE=false
  write_runtime
  trap zero_key EXIT HUP INT TERM
  compose pull
  compose up -d server inspector-tailscale inspector exporter udp-helper
  wait_healthy server
  wait_healthy inspector-tailscale
  wait_exporter
  wait_runtime_counts 3 0
  zero_key
  trap - EXIT HUP INT TERM
  echo "immutable exporter dogfood enrolled; auth key file zeroed"
  echo "private evidence: $TAILPATH_EXPORTER_DOGFOOD_EVIDENCE"
}

restart_server() {
  degraded_before=$(exporter_log_count "exporter transport degraded")
  recovered_before=$(exporter_log_count "exporter transport recovered")
  restore_server() {
    compose start server >/dev/null 2>&1 || true
  }
  trap restore_server EXIT HUP INT TERM
  compose stop server
  wait_exporter_log_increment "exporter transport degraded" "$degraded_before"
  sleep 30
  compose start server
  wait_healthy server
  wait_exporter_log_increment "exporter transport recovered" "$recovered_before"
  wait_runtime_counts 3 0
  degraded_after=$(exporter_log_count "exporter transport degraded")
  recovered_after=$(exporter_log_count "exporter transport recovered")
  test "$degraded_after" -eq $((degraded_before + 1)) \
    || fail "server outage produced more than one degraded transition"
  test "$recovered_after" -eq $((recovered_before + 1)) \
    || fail "server outage produced more than one recovered transition"
  trap - EXIT HUP INT TERM
}

server_outage() {
  wait_path direct
  capture before-server-outage >/dev/null
  restart_server
  wait_path direct
  capture after-server-outage >/dev/null
  assert_continuity before-server-outage after-server-outage
}

restart_exporter() {
  export TAILPATH_EXPORTER_DOGFOOD_LIFECYCLE=false
  compose up -d --force-recreate exporter udp-helper
  wait_exporter
  wait_runtime_counts 3 0
}

assert_continuity() {
  before=${1:-}
  after=${2:-}
  for scenario in "$before" "$after"; do
    case "$scenario" in
      ""|*[!A-Za-z0-9_-]*|?????????????????????????????????*)
        fail "continuity scenarios must use 1-32 safe ASCII characters"
        ;;
    esac
  done
  before_raw="$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE/$before-topology.raw.json"
  after_raw="$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE/$after-topology.raw.json"
  before_safe="$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE/$before-topology.json"
  after_safe="$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE/$after-topology.json"
  before_history_raw="$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE/$before-history.raw.json"
  after_history_raw="$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE/$after-history.raw.json"
  after_history="$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE/$after-history.json"
  for required in "$before_raw" "$after_raw" "$before_safe" "$after_safe" \
    "$before_history_raw" "$after_history_raw" "$after_history"; do
    test -f "$required" || fail "missing capture file for continuity check"
  done
  host_a="${TAILPATH_EXPORTER_DOGFOOD_PREFIX}-runtime-a"
  host_b="${TAILPATH_EXPORTER_DOGFOOD_PREFIX}-runtime-b"
  edge_query='[.nodes[] | select(.hostname == $hostA)][0].id as $a
    | [.nodes[] | select(.hostname == $hostB)][0].id as $b
    | [.edges[] | select(([.source,.target]|sort) == ([$a,$b]|sort))][0].id'
  before_edge=$(jq -er --arg hostA "$host_a" --arg hostB "$host_b" "$edge_query" "$before_raw")
  after_edge=$(jq -er --arg hostA "$host_a" --arg hostB "$host_b" "$edge_query" "$after_raw")
  test "$before_edge" = "$after_edge" || fail "canonical business edge changed across outage"
  jq -e --argjson before "$(jq '.businessEdge.bytesPerSecond' "$before_safe")" '
    .businessEdge.bytesPerSecond <= ([($before * 4), 16777216] | max) and
    .businessEdge.forwardPositive and .businessEdge.reversePositive
  ' "$after_safe" >/dev/null || fail "post-recovery rate violates the no-catch-up bound"
  jq -e '.trafficPoints > 0 and .directionalTraffic.forwardPositive and .directionalTraffic.reversePositive' \
    "$after_history" >/dev/null || fail "History did not survive continuity check"
  before_to=$(jq -er '.to' "$before_history_raw")
  jq -e --arg before_to "$before_to" '
    def epoch: sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601;
    (.to | epoch) > ($before_to | epoch)
  ' "$after_history_raw" >/dev/null \
    || fail "History capture did not advance across continuity check"
  added_bytes=$(jq --arg before_to "$before_to" '
    def epoch: sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601;
    ($before_to | epoch) as $cutoff
    | [.traffic[]?
      | select((.bucketStart | epoch) >= $cutoff)
      | ((.aToBBytes // 0) + (.bToABytes // 0))
    ] | add // 0
  ' "$after_history_raw")
  test "$added_bytes" -le 67108864 \
    || fail "History growth violates the 64 MiB no-catch-up bound"
  echo "continuity and no-catch-up checks passed"
}

lifecycle() {
  export TAILPATH_EXPORTER_DOGFOOD_LIFECYCLE=true
  export TAILPATH_EXPORTER_DOGFOOD_LIFECYCLE_STEP=60s
  compose up -d --force-recreate exporter udp-helper
  wait_exporter
  wait_runtime_counts 3 0
  host_c="${TAILPATH_EXPORTER_DOGFOOD_PREFIX}-runtime-c"
  before_id=$(api /api/v1/topology | jq -er --arg host "$host_c" '[.nodes[] | select(.hostname == $host)][0].id')
  wait_runtime_counts 2 1
  capture withdrawn
  wait_runtime_counts 3 0
  after_id=$(api /api/v1/topology | jq -er --arg host "$host_c" '[.nodes[] | select(.hostname == $host)][0].id')
  test "$before_id" = "$after_id" || fail "runtime C identity changed after withdrawal and restart"
  capture restarted
}

purge_raw() {
  validate_evidence_dir "$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE"
  find "$TAILPATH_EXPORTER_DOGFOOD_EVIDENCE" -maxdepth 1 -type f \
    \( -name '*.raw.json' -o -name '*.raw.log' \) -delete
  echo "private raw evidence removed"
}

down() {
  udp restore >/dev/null 2>&1 || true
  compose down --volumes --remove-orphans
  zero_key
  echo "project state removed; inspect and delete raw evidence in ${TAILPATH_EXPORTER_DOGFOOD_EVIDENCE:-unknown}"
}

load_for_command() {
  load_runtime
  TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE=$auth_file
  export TAILPATH_EXPORTER_DOGFOOD_AUTHKEY_FILE
}

command=${1:-}
case "$command" in
  up) up ;;
  status) load_for_command; compose ps; compose exec -T server /usr/local/bin/tailpath version ;;
  topology) load_for_command; api /api/v1/topology ;;
  capture) load_for_command; capture "${2:-}" ;;
  wait-path) load_for_command; wait_path "${2:-}" ;;
  udp) load_for_command; udp "${2:-}" ;;
  restart-server) load_for_command; restart_server ;;
  server-outage) load_for_command; server_outage ;;
  restart-exporter) load_for_command; restart_exporter ;;
  lifecycle) load_for_command; lifecycle ;;
  assert-continuity) load_for_command; assert_continuity "${2:-}" "${3:-}" ;;
  purge-raw) load_for_command; purge_raw ;;
  down) load_for_command; down ;;
  *)
    echo "usage: exporter-dogfood.sh <up|status|topology|capture SCENARIO|wait-path direct|derp|udp derp|restore|status|restart-server|server-outage|restart-exporter|lifecycle|assert-continuity BEFORE AFTER|purge-raw|down>" >&2
    exit 2
    ;;
esac
