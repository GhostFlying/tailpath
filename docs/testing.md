# Testing

The test pyramid is deterministic fixtures first, real Tailnet dogfood last.

Go tests currently cover identity and time-valid IP resolution, path
classification and logical transitions, directional endpoint/relay traffic,
counter resets, sparse triggers, control-peer filtering, ordering, lifecycle
aging, reporter ownership transfer, retention, storage-failure atomicity,
eight-day restart recovery, client clock skew, relay-session ingest, and
migration behavior.

The LocalAPI adapter currently has upstream-shaped IPv4 and IPv6 Peer Relay
endpoint cases. Collector integration uses a fake source/reporter, HTTP tests
use an in-process server, and the collector hello is decoded through the
OpenAPI-generated Go model. Frontend unit tests use Vitest.

Playwright covers desktop and Pixel 7 fixture rendering, graph/legend framing,
path filters, the persisted recent option, mobile search, empty activity states,
console errors, and non-overlap. The fixed-seed scale scenario covers 250 nodes,
1,000 logical edges, bilateral provenance, all four paths, active/recent state,
and clock skew. Its Go ingest/restart smoke runs in PR checks; the full desktop
and mobile graph gate runs through the manual `V0.2 release gate` workflow and
uploads JSON and screenshots. Multi-release LocalAPI fixture normalization and
explicit SSE reconnect/conflict/long-label browser cases remain v0.2 work.

The first unoptimized local Docker baseline on 2026-08-23 ingested 750 scale
reports in about 23 seconds and produced a 5.37 MB SQLite database. This records
the starting point for #21; it is not a release performance claim or gate.
Incremental checkpoints and typed state transfer reduced the same ingest to
about 1.88 seconds and the database to 4.04 MB. The browser baseline remains
about 18-19 seconds, isolating layout as #24 work. Server-side 250 ms SSE
coalescing and browser single-flight refresh eliminated the earlier Chromium
`ERR_INSUFFICIENT_RESOURCES` warning; the scale test now fails on any console
error. Browser-local positions and bounded automatic layout reduced the desktop
250-node/1,000-edge `data-ready` measurement below the five-second gate; cached
reloads perform zero automatic layout runs and restore exact coordinates.

The manual scale workflow also builds a steady seven-day history database with
250 nodes, 1,000 continuously active edges, 720,000 raw provenance rows,
2,880,000 minute rows, and 168,000 hour rows. The 2026-08-24 local run produced
707,936,256 bytes in 67.7 seconds, below the 2 GiB target. This is a synthetic
capacity gate rather than a production hardware performance claim.

Manual milestone verification uses at least two real nodes and ordinary
application traffic. Tailpath itself never invokes an active connectivity probe.
Stage one uses the isolated
[real-Tailnet container smoke](runbooks/v0.1-container-smoke.md), including an
opt-in fault that blocks non-DNS UDP only in disposable test namespaces to
validate a real DERP path and transition. The
[constrained-network dogfood runbook](runbooks/v0.1-dogfood.md) remains a
separate cross-host operational exercise. Test-harness fault injection never
changes the passive boundary of production collectors or servers.

The [v0.2 release-gate runbook](runbooks/v0.2-release-gates.md) defines the
immutable alpha artifact record, automated thresholds, Linux container and
native smoke, real arm64 Mac validation, independent review, and final human
release decision. A short local performance run exercises the same container
path but is diagnostic only; it cannot replace the strict hosted workflow or
real-node dogfood.
