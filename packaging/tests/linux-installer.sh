#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT INT TERM
package_dir="$test_dir/package"
install_root="$test_dir/root"
mkdir -p "$package_dir"
cp "$repo_root/packaging/linux/install.sh" \
  "$repo_root/packaging/linux/uninstall.sh" \
  "$repo_root/packaging/linux/tailpath-collector.service" \
  "$package_dir/"
cp /bin/true "$package_dir/tailpath"
chmod 0755 "$package_dir/tailpath" "$package_dir/install.sh" "$package_dir/uninstall.sh"

TAILPATH_INSTALL_ROOT="$install_root" TAILPATH_SKIP_SERVICE=1 \
  "$package_dir/install.sh" --server-url "http://tailpath.example.ts.net:8080" --socket "/run/tailscale/tailscaled.sock"

test -x "$install_root/usr/local/bin/tailpath"
test -f "$install_root/etc/systemd/system/tailpath-collector.service"
test "$(stat -c %a "$install_root/etc/default/tailpath-collector")" = "600"
grep -Fx 'TAILPATH_SERVER_URL="http://tailpath.example.ts.net:8080"' "$install_root/etc/default/tailpath-collector"
grep -Fx 'TAILPATH_RELAY_TELEMETRY="auto"' "$install_root/etc/default/tailpath-collector"
grep -Fx 'TAILPATH_SOCKET="/run/tailscale/tailscaled.sock"' "$install_root/etc/default/tailpath-collector"

printf '\n# operator edit\n' >> "$install_root/etc/default/tailpath-collector"
TAILPATH_INSTALL_ROOT="$install_root" TAILPATH_SKIP_SERVICE=1 \
  "$package_dir/install.sh" --server-url "http://must-not-replace:8080"
grep -Fx '# operator edit' "$install_root/etc/default/tailpath-collector"
! grep -Fq 'must-not-replace' "$install_root/etc/default/tailpath-collector"

TAILPATH_INSTALL_ROOT="$install_root" TAILPATH_SKIP_SERVICE=1 "$package_dir/uninstall.sh"
test ! -e "$install_root/usr/local/bin/tailpath"
test ! -e "$install_root/etc/systemd/system/tailpath-collector.service"
test -f "$install_root/etc/default/tailpath-collector"

TAILPATH_INSTALL_ROOT="$install_root" TAILPATH_SKIP_SERVICE=1 "$package_dir/uninstall.sh" --purge
test ! -e "$install_root/etc/default/tailpath-collector"
