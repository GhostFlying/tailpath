# Data model

Canonical nodes use persisted opaque Tailpath IDs. Identifiers are scoped to
the current Tailnet and include StableNodeID, NodeID, node key, disco key, and
time-valid Tailscale IP. MagicDNS, hostname, and reported operating system are
display/search metadata and never identity aliases.

Resolution prefers StableNodeID, then NodeID/node key, disco key, time-valid
Tailscale IP, and scoped relay session correlation. A relay session client ID,
short disco hint, or underlay endpoint is never inserted into the global alias
map. New strong aliases merge placeholders into the existing opaque node and
migrate current edge references. A current strong
identity can rebind an IP previously associated with a different stable node.
Unresolved identities remain placeholders; hostname-only observations are
invalid.

Strong aliases remain durable. A Tailscale IP alias is refreshed by accepted
runtime evidence and expires after the four-heartbeat node window; an IP seen
again after that point cannot merge an identity into the stale canonical node.
MagicDNS short name is the preferred UI label and remains display metadata, so
renaming it updates presentation without changing canonical identity. A
non-empty OS value from the latest trusted report similarly refreshes the
existing node's device presentation; older collectors may omit it.

Each observable canonical node owns its current inventory generation and peer
membership. A reporter instance owns only its process-local message sequence,
deduplication state, and the current right to update one or more observers. A
new reporter hello transfers that right without moving or discarding observer
inventory, so the complete replacement can withdraw peers removed across a
collector restart. Non-owner inventory, traffic, and heartbeat messages cannot
update the observer.

Explicit observer withdrawal clears only the current reporter ownership and
records a server-received withdrawal time on the observer and its current edge
provenance. The observer becomes offline immediately. Withdrawn provenance is
excluded from Live path and rate reconciliation while the logical edge follows
its ordinary recent expiry. Inventory, canonical identity, redirects, and
History remain durable so a later hello can reclaim the same observer without
inventing catch-up traffic.

A logical edge is an unordered node pair. Directional rates are kept
separately. A-to-B prefers A's TX delta and falls back to B's RX delta, then to
third-party Peer Relay evidence; the values from independent provenance are
never added. The reverse direction applies the symmetric priority.

A relay session names relay observer R and scoped clients A and B. It creates
only the A-B logical edge, with R retained as provenance and as the explicit
relay node. Session ID, VNI, cumulative counters, and identity-resolution status
remain as sanitized provenance. Underlay endpoints exist only while processing
current runtime evidence and are stripped before every durable representation.
Directional counters and source/target resolution status are both normalized
to the canonical unordered edge direction: provenance source describes edge A
and target describes edge B.

The aggregator checkpoints relay scopes by relay canonical ID and VNI. Each
scope keeps bounded session-client bindings and, when available, one fresh
unordered canonical endpoint pair. Session IDs and opaque client IDs remain
nested in that scope; they never become global aliases. Short disco hints and
underlay endpoints are not checkpointed. A scope can infer an unresolved client
only when the other client already identifies one member of the fresh pair.
VNI alone cannot choose either endpoint, and a second incompatible fresh pair
marks the scope conflicted until that evidence ages out. Canonical merges
rewrite all scoped bindings and persist redirects for historical resolution.

Native scoped client IDs remain stable across optional identity enrichment and
underlay endpoint movement whenever a short disco hint is present. The short
hint remains relay-scoped correlation material rather than a global alias. If
the hint is absent, endpoint-derived continuity is intentionally conservative:
an endpoint change creates a fresh baseline and placeholder instead of guessing
that two runtime records are the same client.

Relay clients are `resolved` when strong canonical evidence identifies them,
`partial` when only scoped correlation hints exist, `anonymous` when no identity
hint exists, and `conflict` when evidence points at incompatible canonical
nodes. Missing status in older payloads remains valid.

SQLite stores identity status in a backward-compatible JSON envelope around
the existing node identity. Databases containing the earlier bare identity JSON
decode with an empty status, so v0.3 requires no numbered schema migration.
History node search, edge summaries, and details resolve redirects before
selecting the surviving identity and status.

The latest server-received fresh observation selects `peer_relay`, `direct`,
`derp`, or `unknown`. Path specificity only breaks receive-time ties.
Conflicting fresh observations retain provenance and expose a transitioning
state.

An edge is active for ten seconds after a business byte delta, recent for two
heartbeat intervals, and otherwise hidden. Rates become zero when the active
window ends; historical rates from one side are not reused when the other side
reactivates the edge. System telemetry and heartbeats never advance lifecycle.

Released numbered migrations are append-only. Per-observer ten-second traffic
is retained for one hour, deduplicated logical one-minute traffic for 48 hours,
and logical one-hour traffic for seven days. Minute rollups apply directional
observer preference before summing time buckets and close behind a two-minute
grace watermark. Hour rollup first maps physical aliases and directions into
one logical value per minute, takes the directional maximum for aliases in the
same minute, and then sums distinct minutes. Generated logical hour IDs are
persisted as history aliases so later redirect rebuilds can still resolve them.
Hour rollup and source-tier deletion cannot pass the coverage cursor of the
tier they consume. All retention uses server receive time.

Schema migration 4 removes schema-v3 hour rows and the hour cursor because an
already-aggregated physical hour cannot reveal whether aliases overlapped.
Maintenance rebuilds the recoverable hour range from retained minute rows; it
does not present unreconstructable dogfood totals as exact history.

Canonical merges persist a redirect from the removed opaque ID to the surviving
ID. History resolves redirects before grouping nodes and edges, including
direction reversal when a canonical endpoint order changes. A durable edge map
associates each physical time-series edge with its current logical edge and
orientation. Multiple physical aliases in the same source bucket use the
direction-corrected maximum rather than being summed. Each logical edge with
retained traffic keeps the latest path event across all aliases before the
seven-day cutoff as its window anchor; anchors disappear when the edge has no
retained traffic.

Persisted canonical identity, redirects, current inventory generations, and
latest runtime state outlive time-series retention. A recorded history edge also
outlives its series so a known edge can return an empty selected window without
being confused with an unknown edge ID.

History edge detail exposes the exact server-received `lastTrafficAt` when the
selected window contains traffic. Chart `bucketStart` values remain aggregated
time-axis coordinates and are never used as a substitute for exact recency. A
known edge with no traffic in the selected window omits `lastTrafficAt`.
