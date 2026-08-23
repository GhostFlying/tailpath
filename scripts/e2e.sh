#!/bin/sh
set -eu

api_pid=""
web_pid=""
api_port="${TAILPATH_E2E_API_PORT:-18082}"
admin_port="${TAILPATH_E2E_ADMIN_PORT:-18083}"
cleanup() {
  test -z "$web_pid" || kill "$web_pid" 2>/dev/null || true
  test -z "$api_pid" || kill "$api_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

if test "${TAILPATH_SCALE_E2E:-0}" = "1"; then
  go run ./cmd/tailpath fixture-server \
    --scale \
    --listen="127.0.0.1:$api_port" \
    --admin-listen="127.0.0.1:$admin_port" \
    --web-dir=web/dist > /tmp/tailpath-fixture.log 2>&1 &
else
  go run ./cmd/tailpath fixture-server \
    --listen="127.0.0.1:$api_port" \
    --admin-listen="127.0.0.1:$admin_port" \
    --web-dir=web/dist > /tmp/tailpath-fixture.log 2>&1 &
fi
api_pid=$!
TAILPATH_API_URL="http://127.0.0.1:$api_port" \
  pnpm --dir web exec vite --host 127.0.0.1 --port 5173 > /tmp/tailpath-web.log 2>&1 &
web_pid=$!

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
until curl --fail --silent http://127.0.0.1:5173/ >/dev/null; do
  attempt=$((attempt + 1))
  if test "$attempt" -ge 60; then
    cat /tmp/tailpath-web.log
    exit 1
  fi
  sleep 1
done

if test "${TAILPATH_SCALE_E2E:-0}" = "1"; then
  pnpm --dir web test:e2e scale.spec.ts
else
  pnpm --dir web test:e2e
fi
