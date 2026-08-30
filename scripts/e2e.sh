#!/bin/sh
set -eu

api_pid=""
api_port="${TAILPATH_E2E_API_PORT:-18082}"
admin_port="${TAILPATH_E2E_ADMIN_PORT:-18083}"
fixture_binary="${TAILPATH_E2E_BINARY:-}"
stop_process() {
  pid="$1"
  test -z "$pid" && return
  kill -TERM "-$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}
cleanup() {
  stop_process "$api_pid"
}
trap cleanup EXIT INT TERM

pnpm --dir web build

run_fixture() {
  if test -n "$fixture_binary"; then
    exec setsid "$fixture_binary" "$@"
  else
    exec setsid go run ./cmd/tailpath "$@"
  fi
}

if test "${TAILPATH_SCALE_E2E:-0}" = "1" && test "${TAILPATH_RELAY_SCALE_E2E:-0}" = "1"; then
  echo "TAILPATH_SCALE_E2E and TAILPATH_RELAY_SCALE_E2E are mutually exclusive" >&2
  exit 1
elif test "${TAILPATH_SCALE_E2E:-0}" = "1"; then
  run_fixture fixture-server \
    --scale \
    --devices \
    --listen="127.0.0.1:$api_port" \
    --admin-listen="127.0.0.1:$admin_port" \
    --web-dir=web/dist > /tmp/tailpath-fixture.log 2>&1 &
elif test "${TAILPATH_RELAY_SCALE_E2E:-0}" = "1"; then
  run_fixture fixture-server \
    --relay-scale \
    --devices \
    --listen="127.0.0.1:$api_port" \
    --admin-listen="127.0.0.1:$admin_port" \
    --web-dir=web/dist > /tmp/tailpath-fixture.log 2>&1 &
else
  run_fixture fixture-server \
    --devices \
    --listen="127.0.0.1:$api_port" \
    --admin-listen="127.0.0.1:$admin_port" \
    --web-dir=web/dist > /tmp/tailpath-fixture.log 2>&1 &
fi
api_pid=$!

attempt=0
until curl --fail --silent "http://127.0.0.1:$admin_port/healthz" >/dev/null; do
  attempt=$((attempt + 1))
  if test "$attempt" -ge "${TAILPATH_E2E_STARTUP_ATTEMPTS:-60}"; then
    cat /tmp/tailpath-fixture.log
    exit 1
  fi
  sleep 1
done

attempt=0
until curl --fail --silent "http://127.0.0.1:$api_port/" >/dev/null; do
  attempt=$((attempt + 1))
  if test "$attempt" -ge 60; then
    cat /tmp/tailpath-fixture.log
    exit 1
  fi
  sleep 1
done

if test "${TAILPATH_SCALE_E2E:-0}" = "1"; then
  TAILPATH_E2E_BASE_URL="http://127.0.0.1:$api_port" pnpm --dir web test:e2e scale.spec.ts
elif test "${TAILPATH_RELAY_SCALE_E2E:-0}" = "1"; then
  TAILPATH_E2E_BASE_URL="http://127.0.0.1:$api_port" pnpm --dir web test:e2e relay-scale.spec.ts
else
  TAILPATH_E2E_BASE_URL="http://127.0.0.1:$api_port" pnpm --dir web test:e2e
fi
