#!/bin/sh
set -eu

support_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
config_path="$support_dir/collector.env"
server_url=""
socket=""
relay_telemetry=""

if test -f "$config_path"; then
  while IFS= read -r line || test -n "$line"; do
    case "$line" in
      TAILPATH_SERVER_URL=*) server_url=${line#TAILPATH_SERVER_URL=} ;;
      TAILPATH_SOCKET=*) socket=${line#TAILPATH_SOCKET=} ;;
      TAILPATH_RELAY_TELEMETRY=*) relay_telemetry=${line#TAILPATH_RELAY_TELEMETRY=} ;;
      ""|'#'*) ;;
    esac
  done < "$config_path"
fi

set -- "$support_dir/tailpath" collector
if test -n "$server_url"; then
  set -- "$@" --server "$server_url"
fi
if test -n "$socket"; then
  set -- "$@" --socket "$socket"
fi
if test -n "$relay_telemetry"; then
  set -- "$@" --relay-telemetry "$relay_telemetry"
fi
exec "$@"
