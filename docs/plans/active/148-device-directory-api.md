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

Implemented on `issue/148-device-api`. The generated contract, authenticated
handler, optional capability, topology enrichment, and deterministic fixture
are connected end to end.

## Next step

Open the Draft PR after the full repository gate passes, then build the lazy
Devices workspace in #149.

## Verification

`make generate`, clean generated diff, API tests, fixtures, and `make check`.

## Completion summary

Added `GET /api/v1/devices`, fixed sync and error enums, stable full-directory
responses, runtime evidence, metadata conflicts, and optional topology
enrichment. Directory NodeKeys remain internal and can only match an existing
runtime alias; they are not exposed or installed as directory-only aliases.
The synthetic 250-device fixture includes runtime overlap and directory-only
nodes without changing Live topology.
