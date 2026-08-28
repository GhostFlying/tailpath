# Public exporter contracts

Issue: [#116](https://github.com/GhostFlying/tailpath/issues/116)
Parent: [#113](https://github.com/GhostFlying/tailpath/issues/113)

## Goal

Publish the Tailpath-owned snapshot, report, and HTTP transport contracts that
embedded applications can depend on without importing internal packages,
generated OpenAPI types, or Tailscale implementation types.

## Decisions

- Add the alpha public package `github.com/GhostFlying/tailpath/exporter`.
- Public snapshot values contain normalized Tailpath identities, paths, and
  cumulative counters. They contain no `ipnstate`, LocalAPI, or tsnet types.
- Public report values model protocol v1, including withdrawal and the native
  relay extension, but remain handwritten application contracts rather than
  aliases to generated or internal types.
- `Reporter` exposes capability negotiation and report delivery. The public
  `HTTPReporter` owns direct-Tailnet HTTP defaults and typed compatibility/status
  errors.
- Keep the current native collector behavior unchanged through a temporary
  explicit conversion wrapper. Issue #118 removes that wrapper when migrating
  the collector engine to the public contracts.

## Acceptance

- An external-package test can construct a Snapshot, implement Source and
  Reporter, and use HTTPReporter without importing an internal package.
- Default HTTP transport bypasses environment proxies; custom clients and
  timeouts are preserved.
- Capability, status, malformed-response, and report receipt behavior retain
  typed errors.
- The native collector and all existing packages continue to build and test.

## Current state

Complete. The public `exporter` package owns normalized snapshot, identity,
path, protocol-v1 report, receipt, capability, source, and reporter contracts. Its
HTTPReporter preserves the direct-Tailnet transport policy and typed failures.
External-package compile tests prove consumers need no internal imports, while
the native collector uses a temporary typed conversion wrapper with ordinary
and relay field coverage.

## Next step

No implementation work remains. Archive this plan as part of the #123 v0.4
closeout.

## Verification

Passed before merge:

- `go test -race ./exporter ./internal/collector`
- `go vet ./...`
- `go test ./...`
- `make check`
