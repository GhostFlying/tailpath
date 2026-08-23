#!/bin/sh
set -eu

cleanup() {
  kill "$api_pid" 2>/dev/null || true
}

go run ./cmd/tailpath fixture-server --listen=127.0.0.1:8080 &
api_pid=$!
trap cleanup EXIT INT TERM
pnpm --dir web dev
