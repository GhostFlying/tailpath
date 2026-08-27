# Control traffic visibility

Issue: [#109](https://github.com/GhostFlying/tailpath/issues/109)

## Goal

Keep relay-observed collector delivery traffic in the internal evidence model
without presenting it as user data-plane activity by default.

## Contract

- The server's dedicated tsnet identity is a control node. A logical edge is
  marked `systemTelemetry` when either endpoint is a configured control node.
- System-telemetry edges remain in runtime state, SQLite traffic/history, and
  provenance so operators can audit delivery behavior.
- The Live API returns the classification. The Live UI hides these edges by
  default and offers a persistent `Show Tailpath control traffic` option.
- History continues to query and display the retained edges; no bytes are
  subtracted or inferred from endpoint strings.
- Unresolved relay clients are never guessed to be the control node. Shared
  tailscaled identity remains degraded opt-in per ADR 0003.

## Implementation

1. Add the classification to domain, OpenAPI, generated types, and runtime edge
   state. History retains the existing traffic and provenance records without
   adding a separate filter in this version.
2. Classify relay-derived logical edges against the server control StableNodeID
   set after identity reconciliation, including canonical redirects.
3. Add aggregator, storage/restart, HTTP, and browser tests for control-only,
   mixed, unresolved, and option-toggle cases.
4. Update architecture, security, and issue documentation to describe the
   internal retention and Live default.

## Verification

- `make generate`
- focused Go tests for aggregate/store/httpapi
- web unit and Playwright desktop/mobile checks
- full `make check`

## Current state

The domain/API classification, canonical relay classification, runtime
checkpoint preservation, and Live default filtering are implemented. Control
edges remain available from the topology API and in SQLite/history; the Live
preference is persistent and off by default. Unresolved relay clients are not
classified as control traffic.

Independent review found and the branch now covers ordinary collector control
edges, sticky classification across canonical merge/checkpoint/restore, and
Inspector cleanup when a node becomes hidden. Focused Go tests, 53 web unit
tests, generated-file checks, TypeScript and formatting checks, and the full
desktop/mobile browser suite pass. The browser suite executed 28 tests and
skipped 12 scale or platform-gated cases; the control-traffic screenshots are
included in the CI artifact set.

## Next step

Complete. Control-traffic classification, persistence, Live filtering, and
Inspector behavior were merged and covered by the v0.3 dogfood window.
