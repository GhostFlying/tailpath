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
and mobile graph baseline runs through the manual `Scale baseline` workflow and
uploads JSON and screenshots. Multi-release LocalAPI fixture normalization and
explicit SSE reconnect/conflict/long-label browser cases remain v0.2 work.

The first unoptimized local Docker baseline on 2026-08-23 ingested 750 scale
reports in about 23 seconds and produced a 5.37 MB SQLite database. This records
the starting point for #21; it is not a release performance claim or gate. The
same baseline reached browser `data-ready` in about 18 seconds. Bursts from 250
runtime refreshes can produce Chromium `ERR_INSUFFICIENT_RESOURCES` while the
current client aborts superseded topology fetches. The manual workflow records
that known error but still fails on any other console error; #21 owns SSE
coalescing and browser single-flight refresh.

Manual milestone verification uses at least two real nodes and ordinary
application traffic. Tailpath itself never invokes an active connectivity probe.
Stage one uses the isolated
[real-Tailnet container smoke](runbooks/v0.1-container-smoke.md), including an
opt-in fault that blocks non-DNS UDP only in disposable test namespaces to
validate a real DERP path and transition. The
[constrained-network dogfood runbook](runbooks/v0.1-dogfood.md) remains a
separate cross-host operational exercise. Test-harness fault injection never
changes the passive boundary of production collectors or servers.
