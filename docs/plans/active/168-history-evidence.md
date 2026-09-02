# History evidence remediation before v0.5 dogfood

Issue: [#168](https://github.com/GhostFlying/tailpath/issues/168)
Parent: [#147](https://github.com/GhostFlying/tailpath/issues/147)

## Context

History currently retains Tailpath control traffic without exposing its
classification, selects one logical path by cross-observer receipt order, and
cannot resolve third-party observer identities in edge detail. The resulting
path-event storms and mobile provenance layout block meaningful v0.5 dogfood.

## Goals

- Exclude system telemetry from normal History while retaining explicit API
  diagnostics.
- Reconcile fresh path evidence deterministically and compact equivalent
  retained events.
- Return all node references needed to explain path provenance.
- Present newest events first and use an accessible mobile bottom sheet.

## Non-goals

No observer protocol change, active probing, new chart dependency, virtual-list
dependency, or change to edge freshness windows.

## Decisions

`includeSystemTelemetry` is opt-in and has no Web control. Path state is a
sticky primary plus a sorted conflict set; receipt order cannot choose between
fresh observers. Endpoint evidence outranks relay evidence only after the
previous primary loses support. VNI, session ID, endpoint, and sample time are
provenance details rather than path identity.

## Interfaces

History endpoints accept `includeSystemTelemetry=false`. History summaries and
details expose `systemTelemetry`; details also expose `relatedNodes`, and path
events expose `conflicts`. History node references may expose StableNodeID.

## Delivery

1. Backend/store/API PR: schema migration, existing-event compaction,
   deterministic reconciliation, History filtering, related-node lookup,
   generated types, tests, and durable documentation.
2. Web PR stacked on the backend: newest-first timeline, accurate provenance,
   relay resolution, mobile bottom sheet, browser tests, and screenshots.
3. After both rebase-merge, rerun the scale workflow on the new main SHA,
   deploy its immutable edge image, then begin real OAuth dogfood.

## Verification

Focused race tests, v4 database upgrade fixtures, 500-event compaction,
checkpoint/restart, API compatibility, `make check`, desktop/390px/320px
Playwright, immutable-image scale, and independent read-only review.

## Current state

Issue and active plan opened. Backend implementation is in progress from
`origin/main` at `ef656eec7a00c6b7914b03c4e8cd5438de089c09`.

## Next step

Implement the append-only migration and deterministic path evidence model,
then expose the updated History API and generated types.
