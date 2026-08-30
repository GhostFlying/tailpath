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

Accepted concepts exist with synthetic device names; implementation not started.

## Next step

Begin after generated API contracts.

## Verification

Playwright Chromium fallback, screenshots compared with accepted concepts via
`view_image`, bundle audit, and `make check`.

## Completion summary

Pending.
