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

All three implementation stages are complete in a dependent PR stack.
Canonical positions, cache format, API and passive observation remain unchanged.
The final browser gate exposed and fixed a Cytoscape attribute/resize feedback
loop in WebKit, initial Fit scheduling, hidden-label Fit bounds and dense graph
style invalidation. Atomic style batches and overview geometry reuse preserve
the existing scale thresholds. Unknown markers are symbols, not device names.

## Verification

Canonical dev container: make check passed (Go checks, 71 frontend unit tests,
type/build/format checks and 52 browser cases; 30 inapplicable cases skipped).
Dedicated layout gate: 21 Chromium and 21 WebKit cases, with one desktop-only
skip per engine. Covers five viewports, cold/cache, Fit, Relayout, selection,
Unicode names, late font events, rate updates, responsive workspaces and routes.
Browser plugin not available; regular Playwright used. Synthetic screenshots
visually inspected. Real iOS and hardware keyboard behavior remain unverified.

Production-build scale: 250 canonical nodes / 1,000 logical edges / 505 rendered
nodes. Desktop cold ready 4,455ms, cached 5,160ms, API response 134ms and visible
rate update 454ms. Mobile cold 4,608ms, cached 4,450ms. Both cases passed; desktop
thresholds remain 5,000ms cold and 500ms update. Separate 1,000-session Peer Relay
scale: desktop and mobile passed. Timings are local measurements, not hardware
performance guarantees. Detailed geometry diagnostics are opt-in build output.

Reproduction: make check; TAILPATH_SCALE_E2E=1 ./scripts/e2e.sh;
TAILPATH_RELAY_SCALE_E2E=1 ./scripts/e2e.sh;
VITE_LAYOUT_DIAGNOSTICS=1 TAILPATH_LAYOUT_E2E=1 ./scripts/e2e.sh.
Install Chromium and WebKit via Playwright in the dev container first.
The layout gate uses one worker: four concurrent WebKit jobs on shared CI
exceeded the settled-state deadline despite passing locally. Assertion and
performance deadlines are unchanged. Use fresh
fixture processes for mutations; scale runs should be isolated from other tests.

## Next step

Complete CI and human review of the dependent PR stack. No automatic merge or
release. Primary checkout edits from the planning session remain preserved.

## Completion summary

Implemented geometry/presentation, responsive workspace rules, bilingual docs,
independent browser gates and CI artifact upload. The plan stays active until
review and merge; implementation does not depend on changing runtime APIs.

## Authorized merge and deployment

The user authorized sequential merges and local deployment on 2026-09-12.
The automated review finding for 44px Retry, Next page and View in Live actions
is fixed. Production build/type/format checks passed; focused Chromium browser
checks passed 31 cases (9 inapplicable skips), including error recovery and
rendered action bounds. PRs 180 and 181 are merged; finish PR 182 and deploy.
Retain the existing deployment volume/identity and previous image/config.
