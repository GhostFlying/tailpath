#!/bin/sh
set -eu

harness="scripts/e2e.sh"
config="web/playwright.config.ts"

grep -F 'pnpm --dir web build' "$harness" >/dev/null
grep -F 'http://127.0.0.1:$api_port/' "$harness" >/dev/null
grep -F 'TAILPATH_E2E_BASE_URL="http://127.0.0.1:$api_port"' "$harness" >/dev/null
grep -F 'process.env.TAILPATH_E2E_BASE_URL' "$config" >/dev/null

if grep -F 'vite --host' "$harness" >/dev/null; then
  echo "browser acceptance must not start the Vite development proxy" >&2
  exit 1
fi
