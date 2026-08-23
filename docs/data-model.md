# Data model

Canonical nodes use persisted opaque Tailpath IDs. Identifiers are scoped to
the current Tailnet and include StableNodeID, NodeID, node key, disco key, and
time-valid Tailscale IP. MagicDNS and hostname are display/search metadata and
never identity aliases.

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
renaming it updates presentation without changing canonical identity.

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

Released migrations are append-only. Default report, traffic, and path-history
retention is seven days by server receive time. Persisted canonical identity,
current inventory generations, and latest runtime state outlive time-series
retention.
