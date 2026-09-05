# Add the Devices workspace

Issue: [#149](https://github.com/GhostFlying/tailpath/issues/149)
Parent: [#147](https://github.com/GhostFlying/tailpath/issues/147)

## Context

Directory devices need a dedicated operational workspace rather than graph
nodes with no traffic.

## Goals

Implement lazy `/devices` and `/devices/:nodeId` routes, desktop table and
inspector, mobile list/detail, deferred search, filters, URL state, and all
loading/status states.

## Non-goals

No card grid, server pagination, new chart dependency, or Live inventory mode.

## Decisions

Runtime observed and control status remain separate dimensions. Navigation is
hidden when unconfigured, while direct access explains the disabled state.

## Interfaces

Devices route bundle, shared navigation capability state, query-state filters,
and responsive inspector/detail.

## Steps

Commit sanitized concepts, implement route/data model/components/styles, add
fixtures and Playwright desktop/390/320 coverage, then write a fidelity ledger.

## Tests

Deep links, nav dimensions, deferred search, platform/control filters, URL
restore, disabled/syncing/stale/empty/error/conflict, 250 rows, and Live count.

## Risks

A large catalog can block Live startup or collapse on narrow screens.

## Current state

Complete on main. The lazy Devices workspace uses the generated v0.5 API. The
fixture exposes 250 synthetic directory devices while preserving existing
Live topology, and real OAuth desktop, 390px, and 320px checks are green.

## Next step

No work remains; v0.5 qualification passed and this plan is archived.

## Verification

Playwright Chromium fallback, screenshots compared with accepted concepts via
`view_image`, bundle audit, and `make check`. The ordinary browser suite passes
38 tests with 14 manual-gate skips. The device-enabled 250-node/1,000-edge scale
gate passes on desktop and mobile without changing topology cardinality.

## Completion summary

Implemented capability-gated navigation, full-response loading with SSE
invalidation, deferred client-side search, URL-backed platform/control filters,
desktop table/inspector, mobile list/full-screen detail, and explicit disabled,
stale, empty, and request-error states. The Devices chunk remains lazy and does
not increase the Live workspace chunk. Copy controls, conflict presentation in
Live, and View in Live are intentionally completed by issue #151.
