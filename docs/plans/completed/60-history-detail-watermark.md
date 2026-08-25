# History detail rollup coverage fix plan

Status: complete
Issue: https://github.com/GhostFlying/tailpath/issues/60
Parent: https://github.com/GhostFlying/tailpath/issues/28
Last updated: 2026-08-25

## Context

History list and node queries use persisted maintenance coverage to choose
between raw, minute, and hour traffic. Edge detail currently chooses those
tiers only from the nominal one-hour and 48-hour retention ages. Maintenance
intentionally retains a finer tier until the next tier is durably covered, so
a delayed or failed maintenance pass can make the list show traffic that edge
detail silently omits.

## Goals

- Make edge detail use the retained finer tier wherever the consuming rollup
  watermark has not reached the nominal tier boundary.
- Preserve fixed-window resolutions and the 200-point response bound.
- Avoid gaps and overlapping tier reads for missing, lagging, and current
  maintenance cursors.

## Decision

Detail queries continue to use nominal retention boundaries because their
time-series resolution differs from summary queries. Each nominal handoff is
additionally capped by the persisted consuming-tier watermark:

- 15m and 1h read raw traffic only.
- 6h and 24h read minute traffic through the earlier of the nominal raw
  boundary and minute coverage, then raw traffic.
- 7d reads hour traffic through the earlier of the nominal minute boundary and
  hour coverage, minute traffic through the earlier of the nominal raw
  boundary and minute coverage, then raw traffic.

The resulting segments are adjacent half-open ranges. No tier overlap is
queried, so directional totals cannot be double-counted.

## Steps

- [x] Add missing and lagging watermark regression fixtures.
- [x] Replace fixed-age detail segmentation with coverage-aware segmentation.
- [x] Verify list/detail directional parity and non-overlapping tier totals.
- [x] Run focused store/API tests and full repository checks.

## Tests

- Missing cursors retain and return older raw traffic in 6h detail.
- Lagging minute coverage reads minute before the cursor and raw after it.
- Lagging hour coverage reads hour, minute, and raw as adjacent ranges in 7d.
- Rows deliberately present in both sides of a completed handoff are counted
  only from the selected tier.

## Current state

Coverage-aware detail segmentation and all regression fixtures are implemented
and merged through PR #61. Missing watermarks now select retained raw rows;
lagging minute and hour watermarks produce adjacent hour/minute/raw ranges whose
directional totals agree with the History list.

## Next step

Retain the coverage-aware list/detail parity fixtures as migration and
maintenance regression coverage.

## Verification record

- Before the fix, the new fixtures reproduced list/detail mismatches of
  `42/0`, `70/340`, and `100/640` bytes for missing, minute-lagging, and
  hour-plus-minute-lagging coverage respectively.
- After the fix, focused store and HTTP API tests pass and all three fixtures
  report identical list/detail directional totals without overlap.
- Generated Go and TypeScript files remain unchanged. Go 1.26.6 formatting,
  vet, all tests, and build pass. Web type/format checks, 46 unit tests, and the
  production build pass.
- Playwright against the built fixture binary passes 13 normal tests with seven
  expected project/viewport skips. The deterministic 250-node/1,000-edge scale
  fixture passes on desktop and mobile.
- The canonical dev-container build could not refresh its Debian package index
  through this host's failing container IPv4 route. Equivalent checks used the
  locked Go image with the existing module cache; hosted CI remains the
  canonical final `make check` result.

## Completion summary

PR #61 was rebase-merged into the final v0.2 candidate. Hosted main CI, the
unchanged strict release gate, and an independent follow-up review all passed;
History list and detail now remain consistent when maintenance coverage is
missing or delayed.
