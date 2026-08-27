# Embedded tsnet source adapter

Issue: [#119](https://github.com/GhostFlying/tailpath/issues/119)
Parent: [#113](https://github.com/GhostFlying/tailpath/issues/113)

## Goal

Expose passive runtime status from an application's existing `tsnet.Server` as
an independent Tailpath Source without leaking upstream status types into the
core exporter contract.

## Decisions

- Publish `exporter/tsnet.New(*tsnet.Server)` and
  `exporter/tsnet.NewLocalClient(*local.Client)`. Each returned Source represents
  exactly one embedded Tailscale identity.
- `New` obtains the server LocalClient at construction. Upstream may start an
  unstarted server while doing so, so applications must configure their server
  first and continue to own its login, readiness, restart, and close lifecycle.
- Snapshot performs only `GET /localapi/v0/status`. It never pings, dials a peer,
  reads packets, edits preferences, or invokes `Up`.
- Share the normalized status-to-snapshot conversion with the native LocalAPI
  adapter. Stable identity, OS normalization, Peer Relay VNI parsing, and path
  precedence therefore cannot drift between native and embedded observers.
- Do not implement Peer Relay server-session telemetry in this adapter. An
  ordinary embedded tsnet runtime exposes only its peer status.

## Acceptance

- Self and peers preserve stable node identity, friendly names, OS, Tailscale
  IPs, counters, and Direct/DERP/Peer Relay/Unknown path attributes.
- A missing self identity, LocalAPI error, nil server, or nil client returns a
  bounded error without leaking status payloads.
- Tests assert the adapter uses only the passive status endpoint and implements
  `exporter.Source` from an external package.
- Native LocalAPI behavior remains unchanged after conversion sharing.

## Current state

`exporter/tsnet` exposes Source constructors for a configured `tsnet.Server`
and an existing `local.Client`. Snapshot reads only the passive status endpoint
and returns bounded non-context failures. Native and embedded adapters now use
one `internal/tailscalestatus` conversion for stable identity, names, OS,
counters, and Direct/DERP/Peer Relay/Unknown path state. External-package and
upstream-shaped tests cover the public contract and passive request boundary.

## Verification

- `go test -race -count=20 ./exporter/tsnet ./internal/tailscalestatus ./internal/tailscaleadapter`
- `go vet ./...`
- `go test ./...`
- `make check`
