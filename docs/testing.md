# Testing

The test pyramid is deterministic fixtures first, real Tailnet dogfood last.

Go tests currently cover identity and time-valid IP resolution, path
classification and logical transitions, directional endpoint/relay traffic,
counter resets, sparse triggers, control-peer filtering, ordering, lifecycle
aging, reporter ownership transfer, retention, storage-failure atomicity,
eight-day restart recovery, client clock skew, relay-session ingest, and
migration behavior.

The LocalAPI adapter has upstream-shaped IPv4 and IPv6 Peer Relay endpoint
cases and a fake transport restricted to passive relay-session and disco-key
reads. Adapter tests distinguish unsupported, disabled, enabled, transient,
malformed, reordered, and ambiguous-disco inputs; they assert stable scoped
IDs, directional counters, and unique full-disco enrichment. Collector
integration uses a fake source/reporter, HTTP tests use an in-process server,
and the collector hello is decoded through the OpenAPI-generated Go model.
Frontend unit tests use Vitest.

Playwright acceptance runs against a fresh `web/dist` bundle served directly
by the fixture server on the same origin as `/api`. Vite remains a development
tool and is intentionally outside ordinary, scale, and relay-scale browser
gates; this avoids development-only StrictMode request replacement interacting
with a proxy while exercising the assets shipped in the image.

Sparse graph entry assertions poll until the viewport converges around the
current positions, so they cannot consume readiness or viewport diagnostics
left by Cytoscape initialization before the scheduled fit completes. The graph
keeps this initial focus pending across intervening topology refreshes and only
clears it after the fit actually runs.

Peer Relay identity-stability tests move one session from unavailable to
available disco enrichment, change an underlay endpoint under the same short
hint, reverse full-identity lexical order, and remove the short hint entirely.
They prove enrichment and ordinary endpoint movement preserve scoped IDs and
direction, while endpoint-only identity changes establish a new baseline. A
same-session short-hint collision proves the endpoint fallback keeps both
clients distinct and direction deterministic.
Forbidden, failed, malformed, and transport-failed optional enrichment keeps
readable session counters and emits only bounded degraded/recovered state.

Sparse relay collector tests prove that first samples, resets, removals,
reappearance, LocalAPI degradation, report outages, and reconnects do not
create synthetic catch-up traffic. They also inject endpoint, session, and
disco text into adapter errors and assert that collector logs do not contain
those details. Native Linux and macOS installer fixtures preserve the explicit
auto/off configuration path; the hosted Windows matrix covers its PowerShell
runner.

Relay reconciliation tests cover stable anonymous clients across repeated
updates and checkpoint restore, strong two-sided resolution, endpoint evidence
arriving before or after a relay session, one-sided inference, VNI-only
ambiguity, conflicting fresh endpoint pairs, scoped-identifier isolation, and
endpoint-over-relay directional traffic precedence. Race tests cover the
aggregator and atomic application commit path.

Cross-layer enrichment coverage proves an anonymous scoped client merges into
its canonical node with a durable redirect, one logical edge, and stable
checkpoint/restart state. A reversed canonical edge test verifies that both
traffic direction and source/target identity status are swapped together.

Durable relay tests scan the SQLite database and WAL for synthetic endpoint and
disco canaries, exercise checkpoint restart before and after a scoped merge,
and query the removed physical edge through its redirect. A seven-day test
runs third-party relay traffic through real minute/hour rollup and retention,
then verifies directional totals and the sanitized pre-window path anchor.

The separate fixed-seed relay scale scenario sends 1,000 concurrent
third-party sessions through eight relay runtimes while retaining 250 canonical
nodes and 1,000 client-to-client logical edges. It exercises real
`relay_session_update` reconciliation, atomic SQLite ingest, checkpoints,
restart, History provenance, and directional de-duplication. Database, WAL,
topology, History, and logs are scanned for documentation-range endpoint and
disco canaries.

Playwright covers desktop and Pixel 7 fixture rendering, graph/legend framing,
path filters, the persisted recent option, mobile search, empty activity states,
console errors, and non-overlap. The fixed-seed scale scenario covers 250 nodes,
1,000 logical edges, bilateral provenance, all four paths, active/recent state,
and clock skew. Its Go ingest/restart smoke runs in PR checks; the full desktop
and mobile graph gate runs through the manual `V0.3 Peer Relay scale gate`
workflow and uploads JSON and screenshots. The same workflow runs the separate
1,000-session relay browser gate without replacing the mixed-path baseline.

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

The first local ten-minute constrained gate exposed sustained SQLite and
runtime-copy growth: although all 75,000 reports were accepted, ingest p95 was
589 ms, scheduler lag reached 226 seconds, and a read-only-container history
query failed while creating a disk temporary file. WAL/NORMAL connection
settings, memory-backed SQLite temporary storage, bounded recent report-ID
deduplication, rollup-backed history summaries, and edge-filtered detail queries
fixed those causes. The 2026-08-24 rerun accepted 75,000 of 75,000 reports in
599.996 seconds: ingest p95/p99 were 43.3/77.8 ms, scheduler lag 200.7 ms,
topology/list/detail p95 were 16.1/81.2/11.1 ms, and peak process RSS was 45.3
MiB with no 500, OOM, or restart.

The same revision's scale browser run recorded desktop cold ready at 3,386 ms,
visible edge-only SSE update at 464 ms, cached ready at 2,060 ms, and mobile
cold ready at 4,138 ms. Both projects rendered 250 topology nodes, 1,000
logical edges, and 505 graph elements with no console errors or layout movement.

The first local 1,000-session relay browser run on 2026-08-26 recorded desktop
cold ready at 2,610 ms and cached ready at 2,186 ms; mobile cold ready was 2,376
ms and cached ready was 1,797 ms. Both projects rendered 250 canonical graph
nodes and 1,000 Peer Relay logical edges from eight relay observers with no
console errors. The SQLite/checkpoint restart and privacy smoke completed in
about 0.65 seconds on the development host. These are regression baselines,
not production hardware performance claims.

The v0.3 hosted strict workflow on 2026-08-26 retained the unchanged v0.2
limits. Its successful attempt accepted 75,000 of 75,000 reports over 599.996
seconds at 125 reports/s with no rejected receipt, request error, HTTP 500,
OOM, or restart. Ingest p95/p99 were 49.4/74.2 ms, scheduler lag was 45.7 ms,
topology/list/detail p95 were 10.1/212.2/8.3 ms, and peak process RSS was 58.9
MiB. The relay restart/privacy smoke completed in 0.51 seconds. The seven-day
database contained 720,000 raw, 2,880,000 minute, and 168,000 hour rows in
707,948,544 bytes, below the 2 GiB gate. Both mixed-path and relay-specific
desktop/mobile browser gates passed.

The preceding hosted attempt accepted all 75,000 reports without an HTTP or
storage failure but exceeded ingest and scheduler latency limits. The same
commit then passed unchanged, while local 30-second A/B diagnostics also
passed. It is retained as an unreproduced hosted-runner outlier; no threshold
was relaxed and the successful rerun does not erase the failed evidence.

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
