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

Current topology is served from memory. Every accepted report, traffic bucket,
and logical path transition is committed in one SQLite transaction. A typed
candidate state is checkpointed immediately once and then at most once per
second during ordinary updates. A report that allocates or merges a canonical
node forces an immediate checkpoint so journal replay can never regenerate a
different canonical ID; the checkpoint records the last represented report
rowid. Only after a successful transaction does ingest transfer candidate
ownership and publish an SSE invalidation. Per-client invalidations are
coalesced into 250-millisecond windows, and the browser merges a refresh burst
into one in-flight request plus one follow-up. Storage failure therefore cannot
advance in-memory sequence, inventory, or canonical identity state.

Path transitions compare logical path identity. Observer-local direct endpoints
remain provenance attributes and do not create a new transition when opposite
observers report different ends of the same Direct connection. DERP region and
Peer Relay node changes remain logical transitions. A known Peer Relay node is
retained in the visible topology for as long as fresh edge provenance refers to
it.

Restart restores current reporter sequences, observer-owned inventory
generations and memberships, reporter-to-observer ownership, identity aliases,
nodes, observations, and edge lifecycle from the latest checkpoint, then
replays only reports with a later rowid and writes a new checkpoint. A new
reporter process claims an observer with a complete hello; ordinary messages
from an old session cannot take ownership back. Minute maintenance removes only
raw reports covered by a committed checkpoint. SQLite also stores ten-second
per-observer traffic for one hour, deduplicated logical minute traffic for 48
hours, logical hour traffic for seven days, and aggregated path transitions
with provenance. Rollup and source-tier deletion share one transaction and only
process buckets behind durable coverage watermarks. Minute buckets remain open
for a two-minute late-arrival grace period; hour rollup cannot pass complete
minute coverage, and raw/minute deletion cannot pass the cursor of the tier
that consumes it.

History node, edge-list, and edge-detail APIs expose only fixed windows. Queries
join a persisted physical-to-logical edge map built from canonical redirects,
correct direction before deduplicating alias buckets, use keyset pagination,
and cap detail responses at 200 traffic points and 500 path transitions. A path
anchor records the latest logical-edge state across all aliases at the start of
a window without replaying topology.
