# Scoped Peer Relay client protocol

Issue: [#90](https://github.com/GhostFlying/tailpath/issues/90)

## Goal

Extend protocol version 1 so a Peer Relay can report traffic between clients
that may not yet have globally resolvable Tailnet identities. Keep ordinary
observer reports wire-compatible and make the session-local identity boundary
explicit before collector and reconciliation work begins.

## Decisions

- A relay session client always has a non-empty `sessionClientId`, scoped to
  that one session report. It may also carry a full `NodeIdentity`, a short
  disco hint, and a current underlay endpoint.
- Only the optional full identity is canonical evidence. Session client IDs,
  short disco hints, and endpoints never become global identity aliases.
- Relay VNI is an unsigned 24-bit value and is copied into the Peer Relay path
  plus sanitized provenance.
- Resolution status is `resolved`, `partial`, `anonymous`, or `conflict`.
  Topology and History may expose it, while omission remains compatible with
  v0.2 payloads.
- Sanitized relay provenance exposes session ID, VNI, and the resolution status
  of both clients. It does not expose underlay endpoints or short disco hints.
- This PR defines contract and validation only. The relay adapter, sparse
  collection, canonical reconciliation, and persistence sanitization remain in
  their dedicated issues.

## Implementation

1. Add the relay client, identity status, relay provenance, and VNI schemas to
   OpenAPI and regenerate Go and TypeScript models.
2. Mirror the contract in Tailpath-owned domain types and validate relay
   StableNodeID, distinct scoped clients, 24-bit VNI, positive sample duration,
   and at least one positive directional delta.
3. Adapt existing placeholder relay aggregation tests to the explicit client
   wrapper without treating scoped identifiers as aliases.
4. Supersede ADR 0005's endpoint-identity assumption and update protocol,
   data-model, and security documentation.

## Verification

- Passed: generated-file reproducibility check.
- Passed: `go test ./...`.
- Passed: `pnpm --dir web check` and 48 Vitest tests.
- Passed: relay validation, generated shape, sanitized journal, and sanitized
  checkpoint coverage.
- Passed: `make check`, including production build and 20 Playwright tests
  with 10 expected project-specific skips.

## Current state

The protocol and domain now model scoped relay clients, 24-bit VNI, identity
status, and sanitized provenance. Existing placeholder aggregation accepts the
new wrapper without treating scoped client identifiers as aliases. Journal and
checkpoint tests prove that underlay endpoints are not durable.

## Next step

Complete. The version-1 scoped relay protocol and generated contract were
merged and are covered by the v0.3 closeout.
