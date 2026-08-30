#!/bin/sh
set -eu

temporary=$(mktemp -d /tmp/tailpath-devices-compose.XXXXXX)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

auth_file="$temporary/tailscale-authkey"
oauth_file="$temporary/devices-oauth-secret"
install -m 0444 /dev/null "$auth_file"
printf '%s\n' 'fixture-secret-value' > "$oauth_file"
chmod 0444 "$oauth_file"

base=$(
  TAILPATH_AUTHKEY_FILE="$auth_file" \
    docker compose -f compose.yaml config --format json
)
printf '%s' "$base" | jq -e '
  .services.server.environment.TS_AUTHKEY == "file:/run/secrets/tailscale-authkey" and
  (.services.server.environment | has("TAILPATH_DEVICES_OAUTH_CLIENT_ID") | not) and
  ([.services.server.secrets[].source] == ["tailscale-authkey"])
' >/dev/null

configured=$(
  TAILPATH_AUTHKEY_FILE="$auth_file" \
  TAILPATH_DEVICES_OAUTH_CLIENT_ID=fixture-client \
  TAILPATH_DEVICES_OAUTH_CLIENT_SECRET_FILE="$oauth_file" \
  TAILPATH_DEVICES_TAILNET=example.test \
    docker compose -f compose.yaml -f compose.devices.yaml config --format json
)
printf '%s' "$configured" | jq -e --arg oauth_file "$oauth_file" '
  .services.server.environment.TAILPATH_DEVICES_OAUTH_CLIENT_ID == "fixture-client" and
  .services.server.environment.TAILPATH_DEVICES_OAUTH_CLIENT_SECRET_FILE == "/run/secrets/tailscale-devices-oauth-client-secret" and
  .services.server.environment.TAILPATH_DEVICES_TAILNET == "example.test" and
  ([.services.server.secrets[].target] | sort) ==
    (["/run/secrets/tailscale-authkey", "/run/secrets/tailscale-devices-oauth-client-secret"] | sort) and
  .secrets["tailscale-devices-oauth-client-secret"].file == $oauth_file
' >/dev/null

if printf '%s' "$configured" | grep -F 'fixture-secret-value' >/dev/null; then
  echo "expanded Compose model contains the OAuth secret value" >&2
  exit 1
fi

if TAILPATH_AUTHKEY_FILE="$auth_file" \
  TAILPATH_DEVICES_OAUTH_CLIENT_SECRET_FILE="$oauth_file" \
  docker compose -f compose.yaml -f compose.devices.yaml config --quiet \
  >"$temporary/missing-client.out" 2>&1; then
  echo "Devices override accepted a missing OAuth client ID" >&2
  exit 1
fi

if TAILPATH_AUTHKEY_FILE="$auth_file" \
  TAILPATH_DEVICES_OAUTH_CLIENT_ID=fixture-client \
  docker compose -f compose.yaml -f compose.devices.yaml config --quiet \
  >"$temporary/missing-secret.out" 2>&1; then
  echo "Devices override accepted a missing OAuth secret path" >&2
  exit 1
fi
