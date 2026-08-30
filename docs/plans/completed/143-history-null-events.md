# Issue 143: Bound empty History detail collections

Status: complete
Issue: https://github.com/GhostFlying/tailpath/issues/143
Last updated: 2026-08-30

## Goal

Prevent History edge deep links from blanking the React application when a
valid traffic window contains no path transitions and the API serializes a
required collection as JSON `null`.

## Scope

- Keep History store results and HTTP responses conformant with the OpenAPI
  array contract for traffic, path events, and provenance observations.
- Normalize legacy nullable History detail collections at the web API boundary.
- Add Go, web unit, and desktop/mobile deep-link regression coverage using the
  observed empty-window response shape.
- Do not change retention, windows, traffic aggregation, or path semantics.

## Implementation

1. Preserve initialized empty slices when the history path set has no events.
2. Normalize required History detail slices before JSON encoding.
3. Normalize nullable legacy fields immediately after `getEdgeHistory` fetch.
4. Verify a direct detail route renders the bounded no-traffic state without
   console or page errors at desktop and mobile widths.

## Current state

The server now preserves empty History slices and normalizes nested provenance
before encoding. The web client also normalizes nullable collections at its API
boundary so it remains compatible with an older server response. Direct detail
routes render the bounded empty state at desktop and mobile widths.

## Next step

Retain the empty-window contract and deep-link browser fixtures as regression
coverage after the fix is merged.

## Verification record

- `go test -count=1 ./internal/store ./internal/httpapi` passes.
- All 55 Vitest tests pass, including nullable API boundary coverage.
- Desktop/mobile Playwright deep-link coverage passes with empty-state
  screenshots and no console or page errors; 30 tests pass and 12
  scale/platform cases skip as designed.
- `make check` passes in the canonical cached devcontainer toolchain.

## Completion summary

The implementation fixes the store assignment that changed an initialized
empty path-event slice back to nil, guarantees array-shaped HTTP output for all
required History detail collections, and keeps the web client tolerant of
older nullable responses.
