# Architecture

[中文版](architecture.zh-CN.md)

```text
tailscaled collector --\
tsnet exporter --------+--> Tailnet HTTP ingest --> aggregator --> SQLite
Peer Relay exporter ---/                              |           |
                                                      +--> SSE --> Web graph
```

Collectors sample locally every two seconds but send traffic messages only
when a non-control peer counter changes. Inventory changes and sparse idle
heartbeats use separate messages.

The server authenticates the reporter connection with Tailscale WhoIs. Trusted
reporters may describe another observer, which lets one tsbridge reporter carry
snapshots for several independent tsnet nodes.

Tailscale types stop at `internal/tailscaleadapter`. Tailpath domain types cross
the protocol, aggregation, storage, and UI boundaries. This protects the wire
and database models from LocalAPI release changes.

The default server is a dedicated tsnet identity. Traffic between a reporter
and this identity is classified as system telemetry, never subtracted from
peer counters, and excluded from user activity.

Current topology is served from memory and committed to SQLite as durable
runtime state after every accepted report. Ingest clones and validates the next
state, persists the report, runtime state, traffic buckets, and logical path
transitions in one transaction, then publishes the committed state to SSE.
Storage failure therefore cannot advance in-memory sequence or inventory state.

Path transitions compare logical path identity. Observer-local direct endpoints
remain provenance attributes and do not create a new transition when opposite
observers report different ends of the same Direct connection. DERP region and
Peer Relay node changes remain logical transitions. A known Peer Relay node is
retained in the visible topology for as long as fresh edge provenance refers to
it.

Restart restores current reporter sequences, observer-owned inventory
generations and memberships, reporter-to-observer ownership, identity aliases,
nodes, observations, and edge lifecycle directly. A new reporter process claims
an observer with a complete hello; ordinary messages from an old session cannot
take ownership back. Raw report retention is not the recovery mechanism. SQLite
also stores ten-second traffic buckets and aggregated path transitions with the
provenance supporting each transition.
