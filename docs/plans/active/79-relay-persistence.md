# Durable Peer Relay provenance

Issue: [#79](https://github.com/GhostFlying/tailpath/issues/79)

## Goal

Retain sanitized Peer Relay identity, path, traffic, and provenance through
checkpoint replay and seven-day History while ensuring relay underlay
endpoints and short disco values never cross a durable storage boundary.

## Decisions

- Sanitize relay reports before the journal transaction. Remove client
  endpoints and replace a non-empty short disco value with a constant presence
  marker so replay preserves `partial` semantics without retaining the hint.
- Checkpoints retain only relay/VNI scope keys, opaque session/client bindings,
  canonical node IDs, and identity status. Runtime endpoint fields remain
  `json:"-"`.
- Store node identity status alongside the existing identity JSON using a
  backward-compatible envelope. Legacy bare `NodeIdentity` payloads decode
  with an empty status, so no numbered migration is required.
- Existing typed path-event JSON already carries only relay stable ID, VNI,
  session ID, and source/target status. Re-encode that owned type at the store
  boundary rather than retaining collector-only relay client fields.
- Existing physical-to-logical edge redirects, third-party directional
  fallback, rollup watermarks, anchors, and retention remain the source of
  truth. Add relay-specific integration coverage instead of a parallel store.

## Implementation

1. Add storage-boundary sanitizers for reports, checkpoints, and path events.
2. Persist and load node identity status without a schema migration.
3. Cover journal/checkpoint/database/WAL scanning with synthetic endpoint and
   disco canaries.
4. Cover restart replay, delayed canonical merge, History node search, path
   anchors/events, rollup fallback, and retention.
5. Update architecture, data model, testing, and parent execution status.

## Verification

- Passed: focused store and application integration tests.
- Passed: database and WAL canary scan with synthetic values.
- Passed: `go test ./...` and store/application race tests.
- Passed: focused mobile History navigation with the real fixture server.
- Partial: `CI=1 make check` passed generated code, shell, Go, TypeScript,
  Vitest, build, and 19 browser tests; the known 30-worker mobile History
  readiness case exhausted its retries and passed immediately with one worker.
- Pending: hosted PR checks.

## Current state

Implementation is complete locally. Journal and checkpoint sanitizers remove
relay endpoints and identifiable short hints; identity status survives legacy
JSON, redirect, restart, rollup, and seven-day History retention.

## Next step

Run the full repository gate, open the stacked PR, and record hosted checks.
