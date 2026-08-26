# ADR 0006: Scope unresolved relay client identity

Status: accepted
Date: 2026-08-26
Supersedes: ADR 0005 endpoint identity requirements

## Context

The Peer Relay status API identifies session participants with relay-local
client positions, short disco hints, and underlay endpoints. It does not always
provide a complete Tailnet node identity. Treating any of those scoped values
as a normal `NodeIdentity` would let unrelated sessions or address reuse merge
different devices into one canonical node.

## Decision

Each relay session endpoint is a `RelaySessionClient` with a required opaque
`sessionClientId` and optional full `identity`, `discoShort`, and `endpoint`.
The client ID is meaningful only within that relay session report. Short disco
hints and endpoints are also correlation hints, never global aliases. Only the
full identity contributes canonical evidence.

The two client IDs must differ. The relay still supplies StableNodeID, and VNI
is restricted to the unsigned 24-bit range. A traffic-bearing report requires a
positive sample duration and at least one positive directional byte delta.

Resolution is exposed as `resolved`, `partial`, `anonymous`, or `conflict`.
Sanitized provenance retains session ID, VNI, and both endpoint resolution
states. Underlay endpoints remain volatile runtime input and are removed before
the report journal, checkpoint, path events, History, API responses, and logs.

## Consequence

Relay-only traffic can be represented without inventing global identities.
Anonymous nodes may later merge when strong evidence arrives, but ambiguous
evidence stays explicit. Implementations need relay-scoped reconciliation state
in addition to the existing global canonical alias map.
