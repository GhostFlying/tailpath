# Systematic layout implementation

Issue: https://github.com/GhostFlying/tailpath/issues/179
Status: active

## Context

The supplied mobile screenshot exposes relay label crowding and tight device
names. The primary coordination checkout was stale; implementation starts from
af9c331, preserving the merged obstacle routing and existing Devices workspace.

## Goals and non-goals

Implement complete screen footprints, deterministic bounded presentation,
readable labels, responsive chrome and independent browser geometry gates.
Preserve canonical identity, positions, cache format and passive observation.
No server API or migration changes; no automatic merge or release.

## Decisions and interfaces

Keep Cytoscape. Separate pure geometry from renderer measurement and scheduling.
Use 12px body clearance, 8px label clearance, 4px path clearance and 16px Fit
padding. Names/rates use 12/11px screen floors. Name budget 160px, DERP 184px.
Use eight label anchors, bounded virtual candidates, spatial indexing and
stable ID tie breaks. Reduce optional labels and expose selection recovery.
Preserve cached/user positions; only new nodes or explicit Relayout may move.
Add internal selection/diagnostic types only. Detailed geometry is test-only.

## Steps and commit boundaries

1. Live geometry, readable presentation, accessible object selection and
   measured legend occupation, with focused tests and a draft PR.
2. Responsive navigation/filter/inspector and History/Devices metadata rules,
   with focused browser checks in a separate coherent commit/PR.
3. Complete synthetic geometry, WebKit and scale gates, bilingual docs and
   make check, in a final validation commit/PR.

## Tests

Use Browser plugin if available; it is absent in this session, so use regular
Playwright. Flow: Live loads -> cached shared DERP topology -> Fit/selection ->
readable non-overlapping names and available controls without position loss.
Test five planned viewports, cold/cache, rate changes, search/reconnect, touch,
keyboard, inspector, browser and graph zoom, plus 250-node/1000-edge scale.
Inspect synthetic screenshots; record WebKit and real iOS separately.

## Risks

Screen-space text affects Fit and routing. Bound repeated layout and avoid
passive camera movement. Dense overview cannot display every name; expose
reduced detail and full inspector/object-list access. Existing obstacle routing
must not regress. Browser timings and late fonts require settled diagnostics.

## Current state

Stage one implemented and focused checks passed. Existing routing and Devices
behavior preserved. Three-pass Fit freezes virtual positions; 420px short-screen
graph minimum avoids shrinking names into unreadable or clipped content.

## Next step

Commit stage one and open draft PR; implement responsive shared workspace rules.

## Verification

Dev-container TypeScript/build and 70 unit tests passed. Synthetic geometry and
existing obstacle browser checks: 15 passed, 1 desktop-only skip. Five viewports,
cold/cache, Fit, Relayout, object selection and clipping covered. Screenshots
inspected; original clipping found and fixed. Full gates pending. Primary
checkout edits are preserved.

## Completion summary

Pending all three stages and required validation.
