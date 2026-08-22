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

Current topology lives in memory and is reconstructed from SQLite after a
restart. SQLite stores latest observations, identity bindings, path events,
ten-second traffic buckets, and relay sessions. Raw two-second status results
and idle heartbeat history are not stored.
