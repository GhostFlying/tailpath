# Multi-observer exporter release gates

Issue: [#121](https://github.com/GhostFlying/tailpath/issues/121)
Parent: [#113](https://github.com/GhostFlying/tailpath/issues/113)

## Goal

Turn the v0.4 multi-runtime exporter invariants into deterministic cross-layer
release gates without adding active probes or a second synthetic topology.

## Decisions

- Reuse the fixed-seed 250-node/1,000-edge scenario. Convert its observer
  snapshots into public exporter inputs so 250 runtimes share one reporter
  sequence and exercise the real 64-observer/1 MiB batching limits.
- Drive the exporter through its public Reporter contract into the real App and
  SQLite store. Verify accepted reports, monotonic sequence, persistence,
  restart, withdrawal, re-registration, and absence of fabricated catch-up
  traffic.
- Keep v0.3 compatibility explicit at the HTTP boundary: a legacy single-
  observer report remains valid without a capabilities request.
- Add a fixture-only observer lifecycle route for Playwright. Production server
  mode never registers the route. The browser gate verifies reporting/stale
  counts and stable coordinates through withdrawal and reconnect.
- Keep the existing strict performance thresholds. This issue adds functional
  coverage for the new ownership shape rather than silently redefining the
  established v0.2/v0.3 capacity contract.

## Acceptance

- Exactly 250 observers and 1,000 bilateral logical edges are produced through
  one reporter instance, with strictly increasing global sequence values,
  batches of at most 64 observers, and requests no larger than 1 MiB.
- A runtime withdrawal is durably visible immediately; re-registration restores
  the same canonical node and edge set without adding offline traffic.
- SQLite restart preserves topology, ownership, redirects, and History data.
- Legacy protocol-v1 single-observer collectors can submit directly without
  capability negotiation.
- Desktop and mobile browser gates preserve node coordinates and layout-run
  count while the runtime summary moves from 250 reporting to 249 reporting +
  1 stale and back to 250 reporting.

## Verification

Passed before merge:

- `go test -race -count=20 ./exporter`
- `go test -race -count=1 ./internal/fixtures ./internal/httpapi ./cmd/tailpath`
- `go vet ./...`
- `go test ./...`
- `make check`
- opt-in Playwright exporter lifecycle scale gate on desktop and mobile

## Current state

Complete. The fixed-seed topology now projects into 250 public exporter sources sharing
one reporter and verifies bounded batches, a global sequence, durable
withdrawal/re-registration, zero reconnect traffic, and SQLite restart. The
scale fixture uses per-reporter sequences, so its isolated edge mutation no
longer creates gaps for unrelated reporters. A fixture-only lifecycle route and
Playwright assertions cover desktop/mobile counts and exact layout stability.
Legacy direct protocol-v1 ingest remains covered without capability preflight.

All Go tests, vet, generated-file checks, shell harnesses, 53 Web unit tests,
production build, 28 default Playwright cases, and the desktop/mobile
250-node/1,000-edge gate pass. Exporter race tests pass for 20 repetitions; the
cross-App/SQLite exporter scale path passes under race instrumentation.

## Next step

No implementation work remains. Archive this plan as part of the #123 v0.4
closeout.
