# Multi-instance tsnet exporter example

Issue: [#120](https://github.com/GhostFlying/tailpath/issues/120)
Parent: [#113](https://github.com/GhostFlying/tailpath/issues/113)

## Goal

Provide a runnable application that proves three independent embedded tsnet
runtimes can report through one dedicated Tailpath transport identity without
system tailscaled.

## Decisions

- Start three runtime `tsnet.Server` instances with independent persisted state
  directories and register each as one observer in a shared SnapshotSink.
- Start a fourth, dedicated reporting `tsnet.Server`. Its HTTP client reaches
  Tailpath and authenticates with WhoIs, but that identity is never registered
  as an observer.
- The application owns all four tsnet lifecycles. The exporter adapter remains
  passive and reads only status; the example never pings or probes peers.
- Accept a reusable auth key directly or through `file:` and never log it. Each
  identity retains its state directory after first enrollment.
- Default to a stable three-runtime process. An explicit `--lifecycle-demo`
  removes the third runtime after one interval and recreates it from the same
  state after the next interval, exercising withdrawal and restart.
- On shutdown, withdraw every registered runtime while the sink is still
  running, then stop the sink and close runtime/reporter servers.

## Acceptance

- Three runtime observers share one monotonic reporter sequence; the dedicated
  reporter identity is absent from observer reports.
- Runtime directories and hostnames are distinct and deterministic.
- Dynamic remove/restart reuses the registration key only after accepted
  withdrawal and closes failed/removed servers.
- Reporter failure, source failure, or shutdown cannot log the auth key or
  fabricate catch-up traffic.
- Unit tests cover lifecycle ordering, rollback, auth-key file handling, and
  configuration validation without contacting a real Tailnet.

## Current state

The runnable example starts three independently persisted runtime identities
and one dedicated reporter identity, then registers only the runtimes with one
SnapshotSink. A tested lifecycle manager orders accepted withdrawal before
close and reuses a registration key only after withdrawal. The opt-in demo
removes and recreates the third runtime from its existing state directory.
Configuration supports environment/flag precedence, reusable `file:` secrets,
an optional control URL, and deterministic generic hostnames.

## Verification

- `go test -race -count=20 ./examples/tsnet-multi`
- `go vet ./...`
- `go test ./...`
- `make check`
