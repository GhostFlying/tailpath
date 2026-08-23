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

install_root=${TAILPATH_INSTALL_ROOT:-}
skip_service=${TAILPATH_SKIP_SERVICE:-0}
if test -n "$install_root"; then
  case "$install_root" in
    /*) ;;
    *) echo "TAILPATH_INSTALL_ROOT must be an absolute path" >&2; exit 1 ;;
  esac
else
  test "$(id -u)" -eq 0 || { echo "uninstall.sh must run as root" >&2; exit 1; }
  test "$(uname -s)" = "Linux" || { echo "uninstall.sh supports Linux only" >&2; exit 1; }
fi

if test "$skip_service" != "1" && command -v systemctl >/dev/null 2>&1; then
  systemctl disable --now tailpath-collector.service >/dev/null 2>&1 || true
fi

rm -f -- \
  "$install_root/usr/local/bin/tailpath" \
  "$install_root/etc/systemd/system/tailpath-collector.service"
if test "$purge" -eq 1; then
  rm -f -- "$install_root/etc/default/tailpath-collector"
fi

if test "$skip_service" != "1" && command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
fi

echo "Tailpath collector uninstalled."
if test "$purge" -ne 1; then
  echo "Preserved configuration: $install_root/etc/default/tailpath-collector"
fi
