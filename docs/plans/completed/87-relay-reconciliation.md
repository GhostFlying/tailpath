# Peer Relay scoped identity reconciliation

Issue: [#87](https://github.com/GhostFlying/tailpath/issues/87)

## Goal

Reconcile relay-scoped clients with canonical Tailnet nodes without inserting
session IDs, short disco hints, or underlay endpoints into the global alias
map, and preserve honest directional traffic when identity is incomplete.

## Decisions

- A relay scope is `relay canonical ID + VNI`. It may hold one fresh unordered
  canonical endpoint pair learned from an endpoint Peer Relay observation or a
  relay session whose clients both carry strong identities.
- A scoped client binding is nested under relay scope and session ID. Its opaque
  client ID maps only to a Tailpath node inside that session.
- Anonymous and partial clients receive stable opaque Tailpath nodes. They do
  not receive fabricated `NodeIdentity` aliases.
- A fresh, non-conflicting scope pair may infer one client only when the other
  session client already matches exactly one member of that pair. VNI-only
  evidence never chooses orientation.
- Contradictory fresh pairs mark the scope conflict and disable inference.
  Strong client identities still resolve independently.
- Canonical node merges rewrite relay scope references, session bindings,
  logical edges, observations, and redirects atomically in candidate state.
- Endpoint observations remain preferred directional evidence. Relay rates and
  history are fallback provenance and are never added to A/B evidence.

## Implementation

1. Parse endpoint-side Peer Relay VNI in the Tailscale adapter.
2. Add checkpointed relay scopes, scoped session bindings, and node identity
   status to aggregator runtime state and typed cloning/restoration.
3. Resolve relay sessions through scoped state, infer only from one matched
   side, and migrate placeholders through canonical merges.
4. Cover anonymous stability, strong resolution, delayed endpoint correlation,
   VNI-only ambiguity, conflicts, restart state, orientation, and A/B/R traffic
   preference.
5. Update architecture, data-model, testing, and parent execution state.

## Verification

- Passed: focused Tailscale adapter tests for bounded IPv4/IPv6 endpoint VNI.
- Passed: aggregator identity, ambiguity, conflict, restart, and fallback tests.
- Passed: aggregator and application race tests.
- Pending: `make check` and PR checks.

## Current state

Implementation is complete locally. Anonymous relay clients now retain scoped
opaque identities across reports and restart; endpoint observations preserve
VNI and can reconcile exactly one missing side without global weak aliases.

## Next step

Complete. Scoped relay reconciliation, conservative identity resolution, and
restart/fallback coverage passed the v0.3 gates.
