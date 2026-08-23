#!/bin/sh
set -eu

purge=0
case "${1:-}" in
  "") ;;
  --purge) purge=1 ;;
  -h|--help)
    echo "usage: uninstall.sh [--purge]"
    exit 0
    ;;
  *) echo "usage: uninstall.sh [--purge]" >&2; exit 2 ;;
esac
test "$#" -le 1 || { echo "usage: uninstall.sh [--purge]" >&2; exit 2; }

skip_service=${TAILPATH_SKIP_SERVICE:-0}
test "$(id -u)" -ne 0 || { echo "uninstall.sh must run as the target macOS user, not root or sudo" >&2; exit 1; }
if test "$skip_service" != "1"; then
  test "$(uname -s)" = "Darwin" || { echo "uninstall.sh supports macOS only" >&2; exit 1; }
  launchctl bootout "gui/$(id -u)/com.tailpath.collector" >/dev/null 2>&1 || true
fi

support_dir="$HOME/Library/Application Support/Tailpath"
logs_dir="$HOME/Library/Logs/Tailpath"
plist_path="$HOME/Library/LaunchAgents/com.tailpath.collector.plist"
config_path="$support_dir/collector.env"

rm -f -- "$plist_path" "$support_dir/tailpath" "$support_dir/run-collector.sh"
if test "$purge" -eq 1; then
  rm -f -- "$config_path" "$logs_dir/collector.log" "$logs_dir/collector.error.log"
  rmdir "$support_dir" "$logs_dir" >/dev/null 2>&1 || true
fi

echo "Tailpath collector uninstalled."
if test "$purge" -ne 1; then
  echo "Preserved configuration: $config_path"
  echo "Preserved logs: $logs_dir"
fi
