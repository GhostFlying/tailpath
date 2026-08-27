# Roadmap

GitHub milestones and issues own execution status. This file owns milestone
intent and exit criteria.

## M0: Foundation

Documented agent workflow, dev container, protocol, code generation, CI,
Compose, fixture server, and an empty live graph that starts from the image.

## v0.1: Vertical slice

Linux LocalAPI collector, sparse traffic reports, control-peer filtering,
identity/path aggregation, SQLite, SSE, and a fixture plus real-node graph.

## v0.2: Usable alpha

Linux server and collector support, macOS and Windows preview packaging,
history, stable layout, reconnect/resync, and the 250-node/1,000-edge
performance gate.

## v0.3: Peer Relay

Relay session telemetry, disco/VNI reconciliation, three-party provenance, and
relay expansion. The existing native collector capability-detects passive relay
server status on Linux. Unresolved clients remain visible as scoped anonymous
nodes and merge only from strong identity evidence; underlay endpoints are
current-state diagnostics and never retained in History. v0.3 exits after a
real relay host passes relay-only, partial-observer, three-party provenance,
restart, path-transition, privacy, and existing scale gates.

## v0.4: tsnet and tsbridge

Public exporter and tsnet adapter packages, an authenticated capability and
withdrawal lifecycle, one process-level multi-observer SnapshotSink, a public
HTTPReporter, native collector reuse, and a runnable multi-instance example.
v0.4 exits after isolated Linux dogfood proves independent runtime identity,
traffic, withdrawal, restart, History, and existing scale behavior.

## v1.0: Stable

Optional Devices API enrichment, stable `/api/v1`, migration compatibility,
cross-platform validation, and security/documentation review.
