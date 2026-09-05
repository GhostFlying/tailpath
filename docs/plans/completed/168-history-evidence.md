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

Complete on main. The backend uses schema v5, compacts equivalent v4 path
events, persists sticky path evidence and system classification, filters
normal History queries, and returns complete provenance node references.
Generated clients and legacy-null normalization are updated.

The Web implementation computes timeline bounds chronologically and renders
them newest-first, resolves observer and relay labels from
`relatedNodes`, distinguishes supporting and conflicting evidence, and uses an
accessible 70dvh mobile bottom sheet with focus and scroll restoration. Browser
coverage includes desktop, 390px-class, and 320px layouts; the first focused
run passed 45 cases with 15 project-conditional skips and produced desktop,
mobile-detail, relay-sheet, and 320px-sheet screenshots. The final canonical
`make check` also passes, including all Go checks, 63 Web unit tests, build, and
45 Playwright cases with 15 project-conditional skips.

## Next step

No work remains; final v0.5 qualification passed and this remediation plan is
archived.

## Completion summary

Normal History excludes system telemetry unless its diagnostic query flag is
explicitly set. Sticky deterministic evidence removes receipt-order path
storms, related node references resolve provenance, and desktop/mobile History
uses newest-first events with an accessible mobile bottom sheet. The final
main scale workflow passed the History API and browser gates.
