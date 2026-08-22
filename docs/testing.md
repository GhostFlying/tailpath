# Testing

The test pyramid is deterministic fixtures first, real Tailnet dogfood last.

Unit tests cover identity resolution, path classification, directional traffic,
counter resets, sparse triggers, control-peer filtering, ordering, retention,
and migration behavior.

Contract tests normalize LocalAPI fixtures from the current and previous
supported Tailscale release. Integration tests use a fake LocalAPI and an
in-process HTTP server. Frontend tests use Vitest and Testing Library.

Playwright covers desktop and mobile graph rendering, empty and conflict states,
SSE reconnection, relay expansion, long labels, and non-overlap. A synthetic
benchmark uses 250 nodes, 1,000 visible edges, and two-second traffic updates.

Manual milestone verification uses at least two real nodes and ordinary
application traffic. Tailpath itself never invokes an active connectivity probe.
