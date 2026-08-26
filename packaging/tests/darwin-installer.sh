#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT INT TERM
package_dir="$test_dir/package"
test_home="$test_dir/home with spaces"
mkdir -p "$package_dir" "$test_home"
cp "$repo_root/packaging/darwin/install.sh" \
  "$repo_root/packaging/darwin/uninstall.sh" \
  "$repo_root/packaging/darwin/run-collector.sh" \
  "$repo_root/packaging/darwin/com.tailpath.collector.plist" \
  "$package_dir/"
cp /bin/true "$package_dir/tailpath"
chmod 0755 "$package_dir/tailpath" "$package_dir/install.sh" \
  "$package_dir/uninstall.sh" "$package_dir/run-collector.sh"

HOME="$test_home" TAILPATH_SKIP_SERVICE=1 "$package_dir/install.sh" \
  --server-url "http://tailpath.example.ts.net:8080" \
  --socket "$test_home/tailscaled.sock"

support_dir="$test_home/Library/Application Support/Tailpath"
logs_dir="$test_home/Library/Logs/Tailpath"
plist_path="$test_home/Library/LaunchAgents/com.tailpath.collector.plist"
test -x "$support_dir/tailpath"
test -x "$support_dir/run-collector.sh"
test "$(stat -c %a "$support_dir/collector.env")" = "600"
grep -Fx 'TAILPATH_RELAY_TELEMETRY=auto' "$support_dir/collector.env"
test -d "$logs_dir"
grep -Fq "$support_dir/run-collector.sh" "$plist_path"
grep -Fq "$logs_dir/collector.log" "$plist_path"

printf '\n# operator edit\n' >> "$support_dir/collector.env"
HOME="$test_home" TAILPATH_SKIP_SERVICE=1 "$package_dir/install.sh" \
  --server-url "http://must-not-replace:8080"
grep -Fx '# operator edit' "$support_dir/collector.env"
! grep -Fq 'must-not-replace' "$support_dir/collector.env"

HOME="$test_home" TAILPATH_SKIP_SERVICE=1 "$package_dir/uninstall.sh"
test ! -e "$support_dir/tailpath"
test ! -e "$plist_path"
test -f "$support_dir/collector.env"

HOME="$test_home" TAILPATH_SKIP_SERVICE=1 "$package_dir/uninstall.sh" --purge
test ! -e "$support_dir/collector.env"
test ! -d "$support_dir"
test ! -d "$logs_dir"
