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

Playwright currently covers desktop and Pixel 7 fixture rendering, graph/legend
framing, path filters, the persisted recent option, mobile search, empty
activity states, console errors, and non-overlap. Multi-release LocalAPI fixture
normalization, explicit SSE reconnect/conflict/long-label browser cases, and the
250-node/1,000-edge synthetic benchmark remain v0.2 gates rather than completed
v0.1 coverage.

Manual milestone verification uses at least two real nodes and ordinary
application traffic. Tailpath itself never invokes an active connectivity probe.
Stage one uses the isolated
[real-Tailnet container smoke](runbooks/v0.1-container-smoke.md), including an
opt-in fault that blocks non-DNS UDP only in disposable test namespaces to
validate a real DERP path and transition. The
[constrained-network dogfood runbook](runbooks/v0.1-dogfood.md) remains a
separate cross-host operational exercise. Test-harness fault injection never
changes the passive boundary of production collectors or servers.
