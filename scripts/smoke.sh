#!/bin/sh
set -eu

repository="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
compose_file="$repository/compose.smoke.yaml"
project="${TAILPATH_SMOKE_PROJECT:-tailpath-smoke}"
default_auth_file=/tmp/tailpath-smoke-authkey
auth_file="${TAILPATH_SMOKE_AUTHKEY_FILE:-$default_auth_file}"
runtime_file="${TAILPATH_SMOKE_RUNTIME_FILE:-/tmp/tailpath-smoke-runtime.env}"
compose_binary="${TAILPATH_COMPOSE:-}"
socket=/var/run/tailscale/tailscaled.sock
server_hostname=tailpath-smoke-server
relay_hostname=tailpath-smoke-r
relay_port="${TAILPATH_SMOKE_RELAY_PORT:-40000}"
udp_chain=TAILPATH-SMOKE-UDP
key_pending=0
compose_auth_file="$auth_file"
TAILPATH_SMOKE_RELAY_IP=""
TAILPATH_SMOKE_SERVER_UNDERLAY_IP=""
TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION="${TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION:-}"

export TAILPATH_SMOKE_AUTHKEY_FILE="$compose_auth_file"
export TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION

compose() {
  if test -n "$compose_binary"; then
    "$compose_binary" -p "$project" -f "$compose_file" "$@"
  else
    docker compose -p "$project" -f "$compose_file" "$@"
  fi
}

fail() {
  echo "tailpath smoke: $*" >&2
  exit 1
}

case "$project" in
  tailpath-smoke|tailpath-smoke-*) ;;
  *) fail "TAILPATH_SMOKE_PROJECT must use the tailpath-smoke prefix" ;;
esac

zero_file() {
  test ! -f "$1" || chmod u+w "$1"
  test ! -f "$1" || : > "$1"
}

zero_pending_key() {
  test "$key_pending" = "1" || return 0
  zero_file "$auth_file"
  if test "$compose_auth_file" != "$auth_file" && test -f "$compose_auth_file"; then
    zero_file "$compose_auth_file"
  fi
}

trap zero_pending_key EXIT
trap 'zero_pending_key; exit 130' HUP INT TERM

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

validate_image_tag() {
  candidate_image_tag="$1"
  test -n "$candidate_image_tag" || fail "ordinary collector image tag must not be empty"
  test "${#candidate_image_tag}" -le 128 || fail "ordinary collector image tag is too long"
  case "$candidate_image_tag" in
    *[!A-Za-z0-9_.-]*) fail "ordinary collector image tag contains invalid characters" ;;
  esac
}

wait_healthy() {
  service="$1"
  attempt=0
  while :; do
    state="$(compose ps "$service" --format json 2>/dev/null \
      | jq -rs '[.[] | if type == "array" then .[] else . end][0].Health // empty' \
        2>/dev/null || true)"
    test "$state" != "healthy" || return 0
    attempt=$((attempt + 1))
    if test "$attempt" -ge 60; then
      compose logs --no-color "$service" >&2 || true
      fail "$service did not become healthy"
    fi
    sleep 2
  done
}

load_runtime() {
  test -f "$runtime_file" || fail "run scripts/smoke.sh up first"
  # This file contains only values generated and validated by this script.
  . "$runtime_file"
  relay_port="$TAILPATH_SMOKE_RELAY_PORT"
  compose_auth_file="$TAILPATH_SMOKE_COMPOSE_AUTH_FILE"
  TAILPATH_SMOKE_AUTHKEY_FILE="$compose_auth_file"
  export TAILPATH_VERSION TAILPATH_SMOKE_SERVER_URL TAILPATH_SMOKE_AUTHKEY_FILE
  export TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION
  export TAILPATH_SMOKE_RELAY_IP TAILPATH_SMOKE_RELAY_PORT
  export TAILPATH_SMOKE_SERVER_UNDERLAY_IP
}

write_runtime() {
  temporary="$(mktemp "${runtime_file}.XXXXXX")"
  chmod 0600 "$temporary"
  {
    echo "TAILPATH_VERSION=$TAILPATH_VERSION"
    echo "TAILPATH_SMOKE_SERVER_URL=$TAILPATH_SMOKE_SERVER_URL"
    echo "TAILPATH_SMOKE_COMPOSE_AUTH_FILE=$compose_auth_file"
    echo "TAILPATH_SMOKE_RELAY_IP=$TAILPATH_SMOKE_RELAY_IP"
    echo "TAILPATH_SMOKE_RELAY_PORT=$relay_port"
    echo "TAILPATH_SMOKE_SERVER_UNDERLAY_IP=$TAILPATH_SMOKE_SERVER_UNDERLAY_IP"
    echo "TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION=$TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION"
  } > "$temporary"
  mv "$temporary" "$runtime_file"
}

stage_compose_key() {
  compose_secret_directory="$(mktemp -d /tmp/tailpath-smoke-secret.XXXXXX)"
  chmod 0700 "$compose_secret_directory"
  compose_auth_file="$compose_secret_directory/tailscale-authkey"

  # Compose bind-mounts local secrets with their host mode. Keep the source in
  # a private directory while allowing the image's nonroot user to read it.
  install -m 0444 "$auth_file" "$compose_auth_file"
  TAILPATH_SMOKE_AUTHKEY_FILE="$compose_auth_file"
  export TAILPATH_SMOKE_AUTHKEY_FILE
}

remove_staged_key() {
  test "$compose_auth_file" != "$auth_file" || return 0
  case "$compose_auth_file" in
    /tmp/tailpath-smoke-secret.*/tailscale-authkey) ;;
    *) return 0 ;;
  esac

  zero_file "$compose_auth_file"
  rm -f "$compose_auth_file"
  rmdir "${compose_auth_file%/tailscale-authkey}" 2>/dev/null || true
}

tailscale_ip() {
  node="$1"
  compose exec -T "tailscale-$node" \
    tailscale --socket="$socket" ip -4 | sed -n '1p'
}

underlay_ip() {
  service="$1"
  container="$(compose ps -q "$service")"
  test -n "$container" || fail "$service is not running"
  docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$container"
}

relay_check() {
  load_runtime
  check="$(compose run --rm --no-deps -T collector-r \
    collector --check --socket="$socket" --relay-telemetry=auto)"
  printf '%s\n' "$check" | jq -e \
    '{relayCapability, relayEnabled, relaySessionCount}
     | select(.relayCapability == "enabled" and .relayEnabled == true)'
}

configure_relay() {
  TAILPATH_SMOKE_RELAY_IP="$(underlay_ip tailscale-r)"
  test -n "$TAILPATH_SMOKE_RELAY_IP" || fail "$relay_hostname has no underlay IPv4"
  compose exec -T tailscale-r tailscale --socket="$socket" set \
    --relay-server-port="$relay_port" \
    --relay-server-static-endpoints="$TAILPATH_SMOKE_RELAY_IP:$relay_port"
  write_runtime
  relay_check >/dev/null || fail "$relay_hostname did not expose relay telemetry"
}

server_ip_from() {
  node="$1"
  compose exec -T "tailscale-$node" \
    tailscale --socket="$socket" status --json \
    | jq -r --arg hostname "$server_hostname" \
      '.Peer[] | select(.HostName == $hostname and .Online == true) | .TailscaleIPs[0]' \
    | sed -n '1p'
}

api_from_a() {
  load_runtime
  container="$(compose ps -q tailscale-a)"
  test -n "$container" || fail "tailscale-a is not running"
  docker run --rm --network "container:$container" \
    "${CURL_IMAGE:-curlimages/curl:8.12.1}" \
    --noproxy '*' \
    --fail --silent --show-error "$TAILPATH_SMOKE_SERVER_URL$1"
}

up() {
  require_command docker
  require_command jq
  case "$auth_file" in /*) ;; *) fail "TAILPATH_SMOKE_AUTHKEY_FILE must be an absolute path" ;; esac
  if test -n "$compose_binary"; then
    test -x "$compose_binary" || fail "$compose_binary is not executable"
  else
    if ! docker compose version >/dev/null 2>&1; then
      fail "docker compose is required; set TAILPATH_COMPOSE to a standalone binary"
    fi
  fi
  test -c /dev/net/tun || fail "/dev/net/tun is required"
  test -s "$auth_file" || fail "write the reusable ephemeral key to $auth_file with mode 0600"
  mode="$(stat -c '%a' "$auth_file")"
  test "$mode" = "600" || fail "$auth_file must have mode 0600, got $mode"
  key_pending=1
  docker info >/dev/null

  cd "$repository"
  if test -n "${TAILPATH_VERSION:-}"; then
    case "$TAILPATH_VERSION" in
      edge-*) candidate_sha=${TAILPATH_VERSION#edge-} ;;
      *) fail "TAILPATH_VERSION must be an immutable edge-<sha> tag" ;;
    esac
    test "${#candidate_sha}" -eq 40 \
      || fail "TAILPATH_VERSION must contain a full 40-character commit SHA"
    case "$candidate_sha" in
      *[!0-9a-f]*) fail "TAILPATH_VERSION commit SHA must be lowercase hexadecimal" ;;
    esac
    TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION="${TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION:-$TAILPATH_VERSION}"
    validate_image_tag "$TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION"
    export TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION
    compose pull server collector-a collector-b collector-r
  else
    TAILPATH_VERSION="smoke-$(git rev-parse --short=12 HEAD)"
    TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION="${TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION:-$TAILPATH_VERSION}"
    validate_image_tag "$TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION"
    export TAILPATH_VERSION
    export TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION
    compose build server
    if test "$TAILPATH_SMOKE_ORDINARY_COLLECTOR_VERSION" != "$TAILPATH_VERSION"; then
      compose pull collector-a collector-b
    fi
  fi
  stage_compose_key
  TAILPATH_SMOKE_SERVER_URL=""
  write_runtime
  compose up -d server tailscale-a tailscale-b tailscale-c tailscale-r
  wait_healthy server
  wait_healthy tailscale-a
  wait_healthy tailscale-b
  wait_healthy tailscale-c
  wait_healthy tailscale-r
  TAILPATH_SMOKE_SERVER_UNDERLAY_IP="$(underlay_ip server)"
  test -n "$TAILPATH_SMOKE_SERVER_UNDERLAY_IP" \
    || fail "$server_hostname has no underlay IPv4"

  attempt=0
  server_ip=""
  until test -n "$server_ip"; do
    server_ip="$(server_ip_from a)"
    attempt=$((attempt + 1))
    test "$attempt" -lt 45 || fail "$server_hostname is not visible from tailscale-a"
    test -n "$server_ip" || sleep 2
  done
  TAILPATH_SMOKE_SERVER_URL="http://$server_ip:8080"
  export TAILPATH_SMOKE_SERVER_URL
  write_runtime

  configure_relay
  compose up -d workload-a workload-b workload-c collector-a collector-b collector-r
  wait_healthy workload-a
  wait_healthy workload-b
  wait_healthy workload-c

  # Enrollment is complete. Keep the mounted file for Compose, but remove the
  # reusable credential value before any workload traffic is generated.
  zero_file "$auth_file"
  zero_file "$compose_auth_file"
  key_pending=0
  echo "tailpath smoke enrolled; auth key file zeroed"
  echo "server=$TAILPATH_SMOKE_SERVER_URL"
  status
}

status() {
  load_runtime
  compose ps
  for node in a b c r; do
    echo "tailpath-smoke-$node $(tailscale_ip "$node")"
  done
  echo "$server_hostname ${TAILPATH_SMOKE_SERVER_URL#http://}"
  echo "$relay_hostname $TAILPATH_SMOKE_RELAY_IP:$relay_port"
  echo "server version $(compose exec -T server /usr/local/bin/tailpath version)"
  echo "ordinary collector version $(compose exec -T collector-a /usr/local/bin/tailpath version)"
}

topology() {
  api_from_a /api/v1/topology
}

traffic() {
  load_runtime
  source_node="${1:-}"
  target_node="${2:-}"
  case "$source_node" in a|b|c) ;; *) fail "source must be a, b, or c" ;; esac
  case "$target_node" in a|b|c) ;; *) fail "target must be a, b, or c" ;; esac
  test "$source_node" != "$target_node" || fail "source and target must differ"
  source_container="$(compose ps -q "tailscale-$source_node")"
  target_ip="$(tailscale_ip "$target_node")"
  test -n "$source_container" || fail "tailscale-$source_node is not running"
  test -n "$target_ip" || fail "tailscale-$target_node has no Tailscale IPv4"
  docker run --rm --network "container:$source_container" \
    "${CURL_IMAGE:-curlimages/curl:8.12.1}" \
    --noproxy '*' \
    --fail --silent --show-error \
    --limit-rate "${TAILPATH_SMOKE_RATE:-2M}" \
    --output /dev/null "http://$target_ip:18088/payload.bin"
}

restart_server() {
  load_runtime
  compose restart server
  wait_healthy server
  api_from_a /api/v1/topology >/dev/null
  echo "tailpath smoke server restarted and API recovered"
}

collector() {
  load_runtime
  action="${1:-}"
  node="${2:-}"
  case "$action" in
    start|stop|restart) ;;
    *) fail "collector action must be start, stop, or restart" ;;
  esac
  case "$node" in a|b|r) ;; *) fail "collector node must be a, b, or r" ;; esac
  compose "$action" "collector-$node"
}

path() {
  load_runtime
  source_node="${1:-}"
  target_node="${2:-}"
  case "$source_node" in a|b|c) ;; *) fail "source must be a, b, or c" ;; esac
  case "$target_node" in a|b|c) ;; *) fail "target must be a, b, or c" ;; esac
  test "$source_node" != "$target_node" || fail "source and target must differ"
  target_ip="$(tailscale_ip "$target_node")"
  result="$(compose exec -T "tailscale-$source_node" \
    tailscale --socket="$socket" status --json \
    | jq -r --arg ip "$target_ip" '
        .Peer[]
        | select(.TailscaleIPs[0] == $ip)
        | if (.PeerRelay // "") != "" then "peer_relay"
          elif (.CurAddr // "") != "" then "direct"
          elif (.Relay // "") != "" then "derp"
          else "unknown"
          end')"
  test -n "$result" || fail "target is not visible from source"
  printf '%s\n' "$result"
}

wait_relay_reporting() {
  attempt=0
  while :; do
    if api_from_a /api/v1/topology \
      | jq -e --arg hostname "$relay_hostname" \
        'any(.observers[]?; .hostname == $hostname and .online == true)' >/dev/null; then
      return 0
    fi
    attempt=$((attempt + 1))
    test "$attempt" -lt 45 || fail "$relay_hostname collector did not resume reporting"
    sleep 2
  done
}

wait_peer_relay_path() {
  attempt=0
  while :; do
    if test "$(path a b)" = "peer_relay" \
      && test "$(path b a)" = "peer_relay"; then
      return 0
    fi
    attempt=$((attempt + 1))
    test "$attempt" -lt 45 || fail "endpoint paths did not return to peer_relay"
    sleep 2
  done
}

restart_relay() {
  load_runtime
  compose restart tailscale-r
  wait_healthy tailscale-r
  # network_mode: service:tailscale-r binds the sidecar to the relay's current
  # network namespace. Reattach it after Docker replaces that namespace.
  compose restart collector-r
  relay_check >/dev/null || fail "$relay_hostname did not recover relay telemetry"
  wait_relay_reporting
  wait_peer_relay_path
  echo "tailpath smoke relay restarted and endpoint paths recovered"
}

udp() {
  load_runtime
  action="${1:-}"
  shift || true
  case "$action" in
    block|relay|restore|status) ;;
    *) fail "udp action must be block, relay, restore, or status" ;;
  esac
  test "$#" -gt 0 || fail "udp action requires at least one node"

  for node in "$@"; do
    case "$node" in a|b|c) ;; *) fail "udp node must be a, b, or c" ;; esac
    compose exec -T "tailscale-$node" sh -s -- \
      "$action" "$udp_chain" "$TAILPATH_SMOKE_RELAY_IP" "$relay_port" \
      "$TAILPATH_SMOKE_SERVER_UNDERLAY_IP" <<'EOF'
set -eu
action="$1"
chain="$2"
relay_ip="$3"
relay_port="$4"
server_ip="$5"

for firewall in iptables ip6tables; do
  command -v "$firewall" >/dev/null 2>&1 || continue
  "$firewall" -S OUTPUT >/dev/null 2>&1 || continue
  case "$action" in
    block)
      "$firewall" -n -L "$chain" >/dev/null 2>&1 || "$firewall" -N "$chain"
      "$firewall" -F "$chain"
      "$firewall" -A "$chain" -p udp --dport 53 -j RETURN
      "$firewall" -A "$chain" -p udp -j REJECT
      "$firewall" -C OUTPUT -j "$chain" >/dev/null 2>&1 \
        || "$firewall" -I OUTPUT 1 -j "$chain"
      ;;
    relay)
      "$firewall" -n -L "$chain" >/dev/null 2>&1 || "$firewall" -N "$chain"
      "$firewall" -F "$chain"
      "$firewall" -A "$chain" -p udp --dport 53 -j RETURN
      if test "$firewall" = iptables; then
        "$firewall" -A "$chain" -p udp -d "$server_ip" -j RETURN
        "$firewall" -A "$chain" -p udp -d "$relay_ip" \
          --dport "$relay_port" -j RETURN
      fi
      "$firewall" -A "$chain" -p udp -j REJECT
      "$firewall" -C OUTPUT -j "$chain" >/dev/null 2>&1 \
        || "$firewall" -I OUTPUT 1 -j "$chain"
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
    test "$action" = "status" || echo "tailpath-smoke-$node udp $action complete"
  done
}

down() {
  if test -f "$runtime_file"; then
    load_runtime
  fi
  for node in a b c r; do
    compose exec -T "tailscale-$node" \
      tailscale --socket="$socket" logout >/dev/null 2>&1 || true
  done
  compose down -v --remove-orphans
  remove_staged_key
  rm -f "$runtime_file"
  if test "$auth_file" = "$default_auth_file"; then
    rm -f "$auth_file"
  fi
  echo "tailpath smoke containers, isolated volumes, and generated runtime file removed"
  echo "the tsnet server is ephemeral and may remain in the control plane briefly"
}

usage() {
  cat <<'EOF'
usage: scripts/smoke.sh <command>

commands:
  up                   enroll and start the isolated smoke topology
  status               show containers and local Tailscale IPs
  topology             print the current topology JSON
  traffic <a|b|c> <a|b|c>
                       transfer a rate-limited 64 MiB file over Tailscale
  collector <start|stop|restart> <a|b|r>
                       control one endpoint or relay collector
  path <a|b|c> <a|b|c>
                       print the passive current path between two endpoints
  relay-check          require enabled relay LocalAPI telemetry
  restart-relay        restart relay tailscaled and verify telemetry recovery
  udp <block|relay|restore|status> <a|b|c> [node ...]
                       control UDP egress in selected test namespaces
  restart-server       restart the tsnet server and verify API recovery
  down                 logout nodes and remove the isolated smoke project
EOF
}

case "${1:-}" in
  up) up ;;
  status) status ;;
  topology) topology ;;
  traffic) shift; traffic "$@" ;;
  collector) shift; collector "$@" ;;
  path) shift; path "$@" ;;
  relay-check) relay_check ;;
  restart-relay) restart_relay ;;
  udp) shift; udp "$@" ;;
  restart-server) restart_server ;;
  down) down ;;
  *) usage; exit 2 ;;
esac
