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
    *) usage; exit 2 ;;
  esac
done

skip_service=${TAILPATH_SKIP_SERVICE:-0}
test "$(id -u)" -ne 0 || { echo "install.sh must run as the target macOS user, not root or sudo" >&2; exit 1; }
if test "$skip_service" != "1"; then
  test "$(uname -s)" = "Darwin" || { echo "install.sh supports macOS only" >&2; exit 1; }
fi
case "$server_url$socket" in
  *'
'*) echo "configuration values cannot contain newlines" >&2; exit 1 ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
binary_source="$script_dir/tailpath"
runner_source="$script_dir/run-collector.sh"
plist_source="$script_dir/com.tailpath.collector.plist"
test -x "$binary_source" || { echo "tailpath binary is missing beside install.sh" >&2; exit 1; }
test -f "$runner_source" || { echo "run-collector.sh is missing beside install.sh" >&2; exit 1; }
test -f "$plist_source" || { echo "LaunchAgent template is missing beside install.sh" >&2; exit 1; }
"$binary_source" version >/dev/null

support_dir="$HOME/Library/Application Support/Tailpath"
logs_dir="$HOME/Library/Logs/Tailpath"
agents_dir="$HOME/Library/LaunchAgents"
binary_path="$support_dir/tailpath"
runner_path="$support_dir/run-collector.sh"
config_path="$support_dir/collector.env"
plist_path="$agents_dir/com.tailpath.collector.plist"

install -d -m 0700 "$support_dir" "$logs_dir"
install -d -m 0755 "$agents_dir"
install -m 0755 "$binary_source" "$binary_path"
install -m 0755 "$runner_source" "$runner_path"

if test ! -e "$config_path"; then
  umask 077
  {
    printf 'TAILPATH_SERVER_URL=%s\n' "$server_url"
    if test -n "$socket"; then
      printf 'TAILPATH_SOCKET=%s\n' "$socket"
    fi
  } > "$config_path"
  chmod 0600 "$config_path"
else
  echo "preserving existing $config_path"
fi

xml_escape() {
  printf '%s' "$1" | sed \
    -e 's/\&/\&amp;/g' \
    -e 's/</\&lt;/g' \
    -e 's/>/\&gt;/g'
}
sed_escape() {
  printf '%s' "$1" | sed -e 's/[\\&|]/\\&/g'
}
runner_xml=$(sed_escape "$(xml_escape "$runner_path")")
stdout_xml=$(sed_escape "$(xml_escape "$logs_dir/collector.log")")
stderr_xml=$(sed_escape "$(xml_escape "$logs_dir/collector.error.log")")
sed \
  -e "s|__TAILPATH_RUNNER__|$runner_xml|g" \
  -e "s|__TAILPATH_STDOUT__|$stdout_xml|g" \
  -e "s|__TAILPATH_STDERR__|$stderr_xml|g" \
  "$plist_source" > "$plist_path"
chmod 0600 "$plist_path"

if ! "$binary_path" collector --check >/dev/null 2>&1; then
  echo "warning: passive LocalAPI check failed; verify that Tailscale is running for this user" >&2
fi

if test "$skip_service" != "1"; then
  domain="gui/$(id -u)"
  launchctl bootout "$domain/com.tailpath.collector" >/dev/null 2>&1 || true
  launchctl bootstrap "$domain" "$plist_path"
  launchctl kickstart -k "$domain/com.tailpath.collector"
fi

echo "Tailpath collector installed."
echo "Configuration: $config_path"
echo "Logs: $logs_dir"
