# Native collector SnapshotSink migration

Issue: [#118](https://github.com/GhostFlying/tailpath/issues/118)
Parent: [#113](https://github.com/GhostFlying/tailpath/issues/113)

## Goal

Run the native tailscaled collector through the same ordinary-observer engine
used by embedded exporters without changing its CLI, environment variables,
installers, passive collection boundary, or Peer Relay behavior.

## Decisions

- Adapt LocalAPI status directly to the public Tailpath-owned exporter snapshot
  contract. The native collector does not retain a second ordinary delta,
  heartbeat, sequence, or reconnect implementation.
- Model Peer Relay sampling as an optional Source capability. Its sampler has
  independent timeout and retry state, while its accepted reports share the
  SnapshotSink capability preflight, reporter sequence, receipts, and transport
  recovery state.
- Relay read or identity-enrichment failures remain isolated from ordinary
  collection. A transport failure or server resync invalidates both ordinary
  and relay baselines so traffic observed during the gap is never reconstructed.
- Keep relay evidence, endpoint strings, and session identifiers out of logs.
  Only capability and enrichment state transitions are logged.
- Keep production timing and request bounds fixed. Tests use the existing
  private SnapshotSink configuration rather than adding native-only knobs to
  the public API.

## Acceptance

- Native ordinary hello, inventory, sparse traffic, heartbeat, reconnect, and
  control-traffic filtering are produced by SnapshotSink.
- Relay and ordinary reports use one monotonically increasing reporter sequence.
- Relay failure does not block ordinary traffic. Relay transport failure or
  resync forces a fresh ordinary hello and a fresh relay baseline without
  catch-up traffic.
- `collector --check`, flags, environment precedence, Linux/macOS/Windows
  packaging, and LocalAPI socket behavior remain unchanged.
- Focused race tests and the full repository gate pass.

## Current state

The native collector now registers its LocalAPI source with the public
SnapshotSink and uses the public HTTPReporter directly. The duplicate ordinary
collector engine and transport conversion layer are removed. Optional Peer
Relay sampling runs independently but submits deltas through the same sink
event loop, so ordinary and relay reports share capability negotiation,
sequence, receipts, and reconnect. Focused repeated race tests cover ordinary
isolation, shared ordering, transport failure, resync, and no catch-up traffic.

## Verification

- `go test -race -count=20 ./exporter ./internal/collector ./internal/tailscaleadapter`
- `go vet ./...`
- `go test ./...`
- `make check`
