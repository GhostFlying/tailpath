# Data model

Canonical nodes use persisted opaque Tailpath IDs. Identifiers are scoped to
the current Tailnet and include StableNodeID, NodeID, node key, disco key, and
time-valid Tailscale IP. MagicDNS, hostname, and reported operating system are
display/search metadata and never identity aliases.

Resolution prefers StableNodeID, then NodeID/node key, disco key, time-valid
Tailscale IP, and relay session correlation. New aliases merge placeholders into
the existing opaque node and migrate current edge references. A current strong
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

A logical edge is an unordered node pair. Directional rates are kept
separately. A-to-B prefers A's TX delta and falls back to B's RX delta, then to
third-party Peer Relay evidence; the values from independent provenance are
never added. The reverse direction applies the symmetric priority.

A relay session names relay observer R and endpoints A and B. It creates only
the A-B logical edge, with R retained as provenance and as the explicit relay
node. Session ID, VNI, endpoint attributes, and cumulative counters remain in
the accepted raw report for later relay-specific correlation.

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
grace watermark. Hour rollup and source-tier deletion cannot pass the coverage
cursor of the tier they consume. All retention uses server receive time.

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
