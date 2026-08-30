# Expose the current device directory

Issue: [#148](https://github.com/GhostFlying/tailpath/issues/148)
Parent: [#147](https://github.com/GhostFlying/tailpath/issues/147)

## Context

The web app needs a stable generated contract for capabilities, sync status,
directory devices, runtime evidence, and metadata conflicts.

## Goals

Add `device-directory`, `GET /api/v1/devices`, TopologyNode enrichment, fixed
status/error enums, stable sort, empty arrays, and SSE invalidation.

## Non-goals

No server pagination, per-device endpoint, observer protocol change, or raw
upstream errors.

## Decisions

Detail deep links reuse the full response. Disabled direct access has an
explicit state; failed first sync is stale plus empty; later failure is stale
plus last-good.

## Interfaces

OpenAPI, generated Go/TypeScript types, capabilities, devices handler, and
topology enrichment.

## Steps

Update OpenAPI, regenerate, implement handlers/conversion, and add 250-device
fixtures.

## Tests

Disabled/syncing/healthy/stale, empty response encoding, sort, conflict fields,
control-connected lastSeen omission, auth, and coalesced SSE.

## Risks

Generated contracts can drift from handlers or expose upstream details.

## Current state

Not started.

## Next step

Begin after synchronizer and persistence behavior stabilize.

## Verification

`make generate`, clean generated diff, API tests, fixtures, and `make check`.

## Completion summary

Pending.
