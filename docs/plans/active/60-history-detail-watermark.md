# History detail rollup coverage fix plan

Status: active
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

- [ ] Add missing and lagging watermark regression fixtures.
- [ ] Replace fixed-age detail segmentation with coverage-aware segmentation.
- [ ] Verify list/detail directional parity and non-overlapping tier totals.
- [ ] Run focused store/API tests and full repository checks.

## Tests

- Missing cursors retain and return older raw traffic in 6h detail.
- Lagging minute coverage reads minute before the cursor and raw after it.
- Lagging hour coverage reads hour, minute, and raw as adjacent ranges in 7d.
- Rows deliberately present in both sides of a completed handoff are counted
  only from the selected tier.

## Current state

The independent post-dogfood review classified the mismatch as a v0.2 release
blocker. The issue and dedicated worktree are open; implementation has not
started.

## Next step

Add regression tests that reproduce list/detail disagreement before changing
the tier selector.
