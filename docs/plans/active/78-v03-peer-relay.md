# v0.3 Peer Relay observability execution

Issue: [#78](https://github.com/GhostFlying/tailpath/issues/78)

## Goal

Deliver passive Peer Relay server telemetry as a production data source. A
Linux relay host uses the existing native collector to report traffic-bearing
sessions; the server reconciles scoped relay clients with canonical Tailnet
nodes and presents honest Live and History provenance without retaining
underlay endpoints.

## Decisions

- Keep observer protocol version 1. Add only relay-specific shapes; ordinary
  v0.2 collector reports remain compatible.
- Extend the existing collector with `relay-telemetry=auto|off`, defaulting to
  capability-detected `auto`; do not install a second service.
- Treat Tailscale v1.102.2 as the tested LocalAPI baseline. Unsupported and
  disabled relay APIs degrade independently from ordinary collection.
- Show unresolved clients as relay- and session-scoped anonymous nodes. A
  short disco hint or underlay endpoint is never a global canonical alias.
- Merge scoped clients only from strong identity evidence plus a consistent
  relay and VNI. VNI-only or conflicting evidence never guesses identity or
  traffic direction.
- Keep underlay endpoints in current in-memory provenance only. Strip them
  before the report journal, checkpoint, path events, logs, and History.
- Enhance Live and History; do not add a Relay workspace.
- Use immutable edge images for dogfood. Tags and releases remain human-only.

## Work items

1. [#88](https://github.com/GhostFlying/tailpath/issues/88): upstream-shaped
   LocalAPI fixtures and passive-boundary tests.
2. [#90](https://github.com/GhostFlying/tailpath/issues/90): scoped relay
   protocol, generated types, validation, and contract documentation.
3. [#86](https://github.com/GhostFlying/tailpath/issues/86): relay session and
   disco identity adapter.
4. [#83](https://github.com/GhostFlying/tailpath/issues/83): sparse relay
   counter collection in the existing collector process.
5. [#87](https://github.com/GhostFlying/tailpath/issues/87): canonical identity,
   VNI correlation, provenance, and traffic fallback.
6. [#79](https://github.com/GhostFlying/tailpath/issues/79): sanitized
   persistence, restart, redirects, and History retention.
7. [#85](https://github.com/GhostFlying/tailpath/issues/85): resolved and
   anonymous relay presentation in Live and History.
8. [#89](https://github.com/GhostFlying/tailpath/issues/89): deterministic
   integration, browser, and 1,000-edge scale gates.
9. [#80](https://github.com/GhostFlying/tailpath/issues/80): immutable-image
   dogfood on a real Linux Peer Relay host.
10. [#82](https://github.com/GhostFlying/tailpath/issues/82): independent
    read-only review, blocker fixes, and milestone closeout.

## Acceptance

- Relay-only traffic creates one client-to-client logical edge with an explicit
  relay node; it never creates relay-to-client business edges.
- Endpoint and relay observations deduplicate without summing A/B/R evidence.
- One strong client match may resolve the other side only when relay and VNI
  agree. Ambiguous evidence remains visible and explicitly unresolved.
- First samples, counter resets, session removal, reconnect, relay restart, and
  server outage never create catch-up traffic.
- No persisted database, WAL, checkpoint, path event, log, fixture, screenshot,
  issue, or PR evidence contains real underlay endpoints, hostnames, Tailnet
  suffixes, or credentials.
- Existing 250-node/1,000-edge, restart, History, SSE, layout, and browser gates
  remain green.
- Real dogfood covers two observable, one observable, and no-collector client
  cases plus Direct to Peer Relay path transitions.

## Current state

The v0.3 milestone, umbrella issue, subsystem issues, and governance PR #91
exist. Issue #88 records Tailscale v1.102.2-shaped disabled, empty, active,
reordered, reset, malformed, unsupported, and ambiguous-disco fixtures behind
a fake LocalAPI transport that allows only passive status reads. No production
collector reads Peer Relay server telemetry yet, endpoint-side VNI is still
discarded, and relay-scoped identity does not exist.

## Next step

Merge PR #91 and the stacked #88 fixture PR, then implement #90 before adapter
or runtime behavior so all subsequent work is grounded in recorded upstream
shapes and a generated Tailpath-owned contract.
