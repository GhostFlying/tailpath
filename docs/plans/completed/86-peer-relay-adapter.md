# Peer Relay LocalAPI adapter

Issue: [#86](https://github.com/GhostFlying/tailpath/issues/86)

## Goal

Read passive Peer Relay server state from the tested Tailscale v1.102.2
LocalAPI shapes and convert it into Tailpath-owned runtime snapshots without
leaking Tailscale types past the adapter boundary.

## Decisions

- `unsupported`, `disabled`, `enabled`, and `transient_failure` are explicit
  capability states. HTTP 404/405 means unsupported; a nil UDP port means
  disabled; transport, permission, and malformed payload failures are
  transient failures returned with an error.
- Missing or unsupported disco-key evidence does not hide usable relay
  sessions. It leaves their clients partial or anonymous.
- A short disco hint enriches a client only when it maps to exactly one full
  disco key and node key. Ambiguous short hints never resolve.
- Session and client ordering is deterministic. Opaque IDs are hashes of
  relay-scoped material and never become canonical aliases.
- Only the two read-only LocalAPI routes recorded by #88 are used. No ping,
  capture, preference, or configuration route is permitted.

## Implementation

1. Add Tailpath-owned relay runtime snapshot types beside collector source
   types.
2. Add the passive LocalAPI reader, typed capability classification, unique
   disco enrichment, stable ordering, and scoped ID generation.
3. Expand the fake transport tests across every fixture and failure class.
4. Update adapter testing documentation and the parent execution state.

## Verification

- Passed: `go test ./internal/tailscaleadapter ./internal/collector`.
- Passed: passive-route, capability, stable-ordering, counter, and identity
  enrichment fixture coverage.
- Passed: `make check`, including production build and 20 Playwright tests
  with 10 expected project-specific skips.

## Current state

Production adapter code now reads both passive routes, emits Tailpath-owned
snapshots, and degrades unsupported disco evidence to unresolved clients. No
collector scheduling or report behavior changed.

## Next step

Complete. The passive relay adapter and its upstream-shaped fixture coverage
were merged as part of the v0.3 implementation.
