# Data model

Canonical nodes use opaque Tailpath IDs. Identifiers are scoped to the current
Tailnet and include StableNodeID, NodeID, node key, disco key, Tailscale IP,
MagicDNS, and hostname.

Resolution order is StableNodeID, NodeID/node key, disco key, time-valid
Tailscale IP, then relay session correlation. Names are display/search fields
and never merge nodes by themselves. Unresolved identities remain placeholders.

A logical edge is an unordered node pair. Directional rates are kept
separately. A-to-B prefers A's TX delta and falls back to B's RX delta; the two
values are never added.

Path kinds are `peer_relay`, `direct`, `derp`, and `unknown`, in that evidence
priority order. Conflicting fresh observations retain provenance and expose a
transitioning state.

An edge is active for ten seconds after a business byte delta, recent for two
minutes, and otherwise hidden by default. System telemetry never advances this
lifecycle.

Released migrations are append-only. Default traffic/path retention is seven
days. Identity and latest metadata outlive time-series retention.
