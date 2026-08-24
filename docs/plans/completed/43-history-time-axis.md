# History traffic time-axis remediation plan

Status: complete
Issue: https://github.com/GhostFlying/tailpath/issues/43
Parent: https://github.com/GhostFlying/tailpath/issues/40
Last updated: 2026-08-24

## Context

The History chart currently distributes returned buckets evenly by array
index. Sparse traffic is therefore stretched across the selected window and
connected continuously, visually claiming activity at times with no sample.
Pointer lookup repeats the same index assumption.

## Decision

- Map every bucket's start/end onto the selected `[from, to)` time domain.
- Render each bucket as a constant-rate step over its real duration. Adjacent
  buckets may connect at their boundary; missing intervals remain visible gaps.
- Do not fill missing buckets with zero because passive sparse reporting cannot
  distinguish confirmed zero traffic from missing observation.
- Pointer lookup returns a tooltip only while the cursor is inside a rendered
  bucket, and tooltip rates remain non-negative real values.
- Keep the stable SVG viewBox, no new chart dependency, and memoized geometry.

## Steps

- [x] Add timestamp-aware, gap-preserving chart geometry.
- [x] Align pointer selection and tooltip placement with real bucket extents.
- [x] Cover sparse, unordered, out-of-window, contiguous, and mobile behavior.
- [x] Run unit, build, desktop/mobile Playwright, and screenshot inspection.
- [x] Update History documentation and record verification.

## Acceptance

- A bucket near the start or end appears at that actual position regardless of
  how many buckets the response contains.
- Two separated buckets produce separate line/area subpaths with a blank gap.
- Hovering the blank gap produces no data tooltip.
- Continuous buckets render as a readable step series without negative rates.
- Desktop and mobile History detail remain unclipped and error-free.

## Current state

History traffic now renders constant-rate step intervals on the selected time
domain. Noncontiguous buckets form separate SVG line/area subpaths, and pointer
lookup returns no point inside a gap. Unit tests cover sparse ordering and
adjacency; 12 applicable desktop/mobile Playwright cases pass, including the
real fixture API and gap hover behavior. Desktop and mobile screenshots were
inspected at original size with no clipping or overlap.

## Next step

No issue-local work remains. Release validation continues in #28.

## Completion summary

PR #50 merged timestamp-positioned sparse traffic geometry, truthful gaps, and
bucket-bounded desktop/mobile tooltips into `main`.
