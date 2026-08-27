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
heartbeats use separate messages. LocalAPI `CollectedAt` is telemetry data time;
idle heartbeat deadlines use the collector process's monotonic scheduling clock
so client wall-clock rollback cannot age out an otherwise healthy observer.

The server authenticates the reporter connection with Tailscale WhoIs. Trusted
reporters may describe another observer, which lets one tsbridge reporter carry
snapshots for several independent tsnet nodes.

The public SnapshotSink gives every registered runtime an isolated sampling
goroutine and gives the process one serialization loop for capability preflight,
batching, reporter sequence, receipts, and reconnect. Production timing and
bounds are fixed: two-second sampling, a fifteen-second source timeout,
100-millisecond batching, at most 64 observers and 1 MiB per request, and
two-to-sixty-second jittered retry. Source failure does not stop siblings. A
transport gap discards delta continuity and retains only each source's latest
snapshot for the next complete hello, so neither memory nor restart can produce
catch-up traffic.

Tailpath-owned exporter contracts live in the public `exporter` package. They
describe normalized snapshots, protocol reports, receipts, and transport
capabilities without exposing generated OpenAPI or Tailscale implementation
types. Tailscale implementation types otherwise stop at
`internal/tailscaleadapter` and `internal/tailscalestatus`; the embedded
adapter's `tsnet.Server` and `local.Client` constructors are the deliberate
`exporter/tsnet` exception. Tailpath domain types cross the protocol,
aggregation, storage, and UI boundaries. This keeps LocalAPI release changes
out of the wire and database models.

`exporter/tsnet` turns either one configured `tsnet.Server` or an existing
embedded `local.Client` into one Source. It reads only LocalAPI status and
shares the same normalization code as the native collector. Obtaining a
LocalClient from an unstarted server may start that server, as defined by
Tailscale; the application still owns login, readiness, restart, and close.
The adapter never calls `Up`, probes peers, or changes preferences.

The default server is a dedicated tsnet identity. Traffic between a reporter
and this identity is classified as system telemetry, never subtracted from
peer counters, and hidden from user activity by default. The classification is
retained in runtime state, SQLite traffic/history, and provenance; Live exposes
a `Show Tailpath control traffic` option for operators who need to inspect it.
Unresolved relay clients are never guessed to be this identity. Sharing a
tailscaled identity is a degraded opt-in mode because its counters cannot
separate control traffic from unrelated applications.

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

Peer Relay session clients are reconciled inside a checkpointed
`relay canonical ID + VNI` scope. A stable endpoint observation can provide a
fresh unordered canonical pair for that scope. The server may resolve the
missing client only when the other client already matches one member of the
pair; VNI-only and conflicting evidence remain anonymous instead of guessing.
Scoped client IDs never enter the global alias index, and canonical merges
atomically rewrite scoped references, current edges, and redirects.

The storage boundary independently sanitizes relay journals and checkpoints.
It removes underlay endpoint fields, replaces journaled short disco values with
a constant presence marker, and re-encodes typed path provenance containing
only relay identity, VNI, session ID, and resolution status. Existing logical
edge mappings apply scoped-node redirects before rollup and query, so relay
fallback traffic and anchors remain attached to the surviving endpoint pair.

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

Dynamic exporter identity replacement first withdraws the previously reported
observer and then sends a complete hello for the replacement on the same
process sequence. Explicit Registration withdrawal retries transient delivery
in memory while the process runs, but there is no durable client queue. A
process crash therefore falls back to the server's ordinary freshness expiry.

History node, edge-list, and edge-detail APIs expose only fixed windows. Queries
join a persisted physical-to-logical edge map built from canonical redirects,
correct direction before deduplicating alias buckets, use keyset pagination,
and cap detail responses at 200 traffic points and 500 path transitions. A path
anchor records the latest logical-edge state across all aliases at the start of
a window without replaying topology.
