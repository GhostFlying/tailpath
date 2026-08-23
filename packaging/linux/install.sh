#!/bin/sh
set -eu

server_url="http://tailpath:8080"
socket=""

usage() {
  echo "usage: install.sh [--server-url URL] [--socket PATH]" >&2
}

while test "$#" -gt 0; do
  case "$1" in
    --server-url)
      test "$#" -ge 2 || { usage; exit 2; }
      server_url=$2
      shift 2
      ;;
    --socket)
      test "$#" -ge 2 || { usage; exit 2; }
      socket=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

install_root=${TAILPATH_INSTALL_ROOT:-}
skip_service=${TAILPATH_SKIP_SERVICE:-0}
if test -n "$install_root"; then
  case "$install_root" in
    /*) ;;
    *) echo "TAILPATH_INSTALL_ROOT must be an absolute path" >&2; exit 1 ;;
  esac
else
  test "$(id -u)" -eq 0 || { echo "install.sh must run as root" >&2; exit 1; }
  test "$(uname -s)" = "Linux" || { echo "install.sh supports Linux only" >&2; exit 1; }
fi

case "$server_url$socket" in
  *'
'*) echo "configuration values cannot contain newlines" >&2; exit 1 ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
binary_source="$script_dir/tailpath"
unit_source="$script_dir/tailpath-collector.service"
test -x "$binary_source" || { echo "tailpath binary is missing beside install.sh" >&2; exit 1; }
test -f "$unit_source" || { echo "tailpath-collector.service is missing beside install.sh" >&2; exit 1; }
"$binary_source" version >/dev/null

binary_path="$install_root/usr/local/bin/tailpath"
config_path="$install_root/etc/default/tailpath-collector"
unit_path="$install_root/etc/systemd/system/tailpath-collector.service"

install -d -m 0755 "$(dirname "$binary_path")" "$(dirname "$unit_path")"
install -d -m 0755 "$(dirname "$config_path")"
install -m 0755 "$binary_source" "$binary_path"
install -m 0644 "$unit_source" "$unit_path"

if test ! -e "$config_path"; then
  umask 077
  {
    printf 'TAILPATH_SERVER_URL="%s"\n' "$(printf '%s' "$server_url" | sed 's/[\\"]/\\&/g')"
    if test -n "$socket"; then
      printf 'TAILPATH_SOCKET="%s"\n' "$(printf '%s' "$socket" | sed 's/[\\"]/\\&/g')"
    fi
  } > "$config_path"
  chmod 0600 "$config_path"
else
  echo "preserving existing $config_path"
fi

if test "$skip_service" != "1"; then
  command -v systemctl >/dev/null 2>&1 || { echo "systemctl is required" >&2; exit 1; }
  systemctl daemon-reload
  systemctl enable --now tailpath-collector.service
fi

echo "Tailpath collector installed."
echo "Configuration: $config_path"
