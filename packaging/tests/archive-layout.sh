#!/bin/sh
set -eu

dist=${1:-dist}

fail() {
  echo "archive-layout: $*" >&2
  exit 1
}

archive_for() {
  os=$1
  arch=$2
  find "$dist" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) |
    awk -v os="$os" -v arch="$arch" '
      BEGIN { IGNORECASE = 1 }
      index(tolower($0), "_" os "_") &&
      ((arch == "amd64" && ($0 ~ /amd64/ || $0 ~ /x86_64/)) ||
       (arch == "arm64" && $0 ~ /arm64/)) { print }
    '
}

list_archive() {
  archive=$1
  case "$archive" in
    *.tar.gz) tar -tzf "$archive" ;;
    *.zip) unzip -Z1 "$archive" ;;
    *) fail "unsupported archive $archive" ;;
  esac | sed 's#^\./##' | LC_ALL=C sort
}

assert_layout() {
  os=$1
  arch=$2
  expected=$3
  matches=$(archive_for "$os" "$arch")
  count=$(printf '%s\n' "$matches" | sed '/^$/d' | wc -l)
  [ "$count" -eq 1 ] || fail "expected one $os/$arch archive, found $count"
  archive=$matches

  actual_file=$(mktemp)
  expected_file=$(mktemp)
  trap 'rm -f "$actual_file" "$expected_file"' EXIT HUP INT TERM
  list_archive "$archive" >"$actual_file"
  printf '%s\n' $expected | LC_ALL=C sort >"$expected_file"
  if ! cmp -s "$expected_file" "$actual_file"; then
    /usr/bin/diff -u "$expected_file" "$actual_file" 2>/dev/null || true
    fail "unexpected layout in $archive"
  fi

  if [ "$os" != windows ]; then
    extract_dir=$(mktemp -d)
    tar -xzf "$archive" -C "$extract_dir"
    [ -x "$extract_dir/tailpath" ] || fail "tailpath is not executable in $archive"
    [ -x "$extract_dir/install.sh" ] || fail "install.sh is not executable in $archive"
    [ -x "$extract_dir/uninstall.sh" ] || fail "uninstall.sh is not executable in $archive"
    rm -rf "$extract_dir"
  fi

  rm -f "$actual_file" "$expected_file"
  trap - EXIT HUP INT TERM
}

archive_count=$(find "$dist" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) | wc -l)
[ "$archive_count" -eq 6 ] || fail "expected six archives, found $archive_count"

for arch in amd64 arm64; do
  assert_layout linux "$arch" \
    "LICENSE README.md install.sh tailpath tailpath-collector.service uninstall.sh"
  assert_layout darwin "$arch" \
    "LICENSE README.md com.tailpath.collector.plist install.sh run-collector.sh tailpath uninstall.sh"
  assert_layout windows "$arch" \
    "LICENSE README.md install.ps1 run-collector.ps1 tailpath.exe uninstall.ps1"
done

(cd "$dist" && sha256sum -c checksums.txt)
echo "archive-layout: six platform archives verified"
