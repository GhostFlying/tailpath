# Directional paths and DERP fallback evidence

## Context

Tailpath currently reconciles all fresh path observations for a logical edge
into one sticky primary plus conflicts. That loses the fact that each endpoint
selects its own outbound path, and it makes a short Tailscale DERP dual-send
window look like an unexplained global path switch. Dense History cards also
cannot accurately reconstruct the two endpoint views or more than 500 path
events.

Issue: https://github.com/GhostFlying/tailpath/issues/188

Accepted concepts:

- Live desktop: `/data00/home/luchengxuan/.codex/generated_images/01a0e6af-dbf8-74c3-809d-de6c457a7d46/exec-be68b0c3-746d-4886-9633-4957efecad54.png`
- History desktop: `/data00/home/luchengxuan/.codex/generated_images/01a0e6af-dbf8-74c3-809d-de6c457a7d46/exec-97770371-b639-44ce-a5e1-2714e0247442.png`
- History mobile: `/data00/home/luchengxuan/.codex/generated_images/01a0e6af-dbf8-74c3-809d-de6c457a7d46/exec-9dcbfb26-f71f-4e83-8856-57c71adf9606.png`

The generated concepts define hierarchy and interaction, not literal OCR copy;
production strings must be accurate and code-native.

## Goals

- Preserve one outbound path state per logical endpoint direction.
- Represent a short Peer Relay to DERP transition as inferred DERP fallback
  without claiming exact Tailscale internals.
- Persist evidence, public relay endpoint/VNI, fallback, and direction so
  post-migration History is reproducible.
- Auto-expand asymmetric Live paths and collapse equivalent directions without
  duplicating logical traffic.
- Replace History cards with two proportional, aligned direction lanes and
  fetch every transition in the selected window.
- Preserve compatibility with existing collectors and API consumers.

## Non-goals

- Active probes, packet capture, route tests, or preference mutation.
- Guessing a missing direction from its reverse.
- Treating relay-session observations as endpoint path votes.
- Retrofitting exact directional transitions that were never stored.
- Forking Tailscale or sing-box in this issue.

## Decisions

1. Endpoint observations own only their outbound direction. Third-party and
   relay-session evidence can enrich relay identity but cannot select a
   direction's active path.
2. `DirectionalPathState` carries source, destination, primary, optional DERP
   fallback, `observed|inferred|legacy` evidence, inference rule, observer, and
   collection/receipt/clock metadata.
3. Existing top-level path, conflicts, state, and candidates remain as
   compatibility projections. New Web surfaces use directions first.
4. A stateful adapter tracker uses a 12-second monotonic window. DERP observed
   shortly after a Peer Relay becomes the relay's inferred fallback; longer
   DERP becomes an inferred DERP primary. Startup DERP is observed without
   guessing a previous relay.
5. `path_events.directions` is an append-only JSON column. Legacy events derive
   best-effort directions from endpoint provenance and are labeled `legacy`.
6. A separate keyset-paginated path-history endpoint removes the existing 500
   event correctness cap while the old embedded detail field remains.
7. Missing directions are unknown. They do not trigger asymmetric expansion.

## Interfaces

Observer peers add optional `fallbackPath`, `pathEvidence`, and
`pathInferenceRule`. Topology edges and path events add `directions[]`.
History adds `GET /history/edges/{edgeId}/paths` with window, cursor, and limit,
returning source/target/related-node references, an anchor, ordered events, and
next cursor. The first page freezes the selected window end in the cursor so
later pages cannot drift as wall time advances.

## Accepted design inventory

- Keep the existing true-white shell, Inter/system sans typography, square
  compact controls, eight-pixel-or-less radii, semantic theme tokens, and
  Cytoscape canvas.
- Primary direction uses the path-kind color and a destination arrow. DERP
  fallback uses amber, a fine dashed route, and explicit `DERP fallback` plus
  evidence text. Color is never the only signal.
- Equivalent directions use the existing single logical route. Different
  primary/fallback identities use two directed route chains and an
  `Asymmetric paths` inspector status.
- The inspector uses two flat direction rows, followed by endpoint evidence
  and relay-session evidence. No nested decorative cards or duplicated totals.
- History uses one shared chronological axis, two primary rails, thin fallback
  subrails, a keyboard/touch-selectable cursor, and an exact-time details table
  or mobile bottom sheet.
- Labels render only when a segment has at least 56 CSS pixels. Short segments
  remain selectable marks and surface exact content through focus, pointer, or
  the details list.
- Preserve 12px unrelated-footprint, 8px label, 4px edge-to-text, 16px viewport,
  and 44x44px touch clearances from the layout contract.
- Motion is limited to selection/focus and a restrained fallback cue, honoring
  `prefers-reduced-motion`.

## Steps

1. Add exporter/domain/OpenAPI contracts, adapter fallback tracking,
   directional reconciliation, append-only storage migration, checkpoint
   behavior, history pagination, compatibility projection, and bilingual docs.
2. Render Live asymmetric routes, fallback overlays, direction inspector rows,
   filters, identity evidence, accessibility, and stable geometry.
3. Implement paginated directional History lanes, exact-time selection,
   desktop/mobile details, dense-segment disclosure, and fixture coverage.
4. Run generated checks, focused Go/Vitest suites, `make check`, and Playwright
   desktop/mobile visual acceptance; compare final screenshots with every
   accepted concept using `view_image`.

## Tests

- Adapter sequences: relay to short DERP to relay, long DERP, startup DERP,
  different relay, monotonic timing, and clearing fallback.
- Aggregation: canonical orientation, reversed edges, 4293/8 asymmetry,
  third-party non-voting evidence, transition equality, and restart checkpoint.
- Store/API: migration, legacy decoding, public endpoint retention, 900-event
  keyset pagination without gaps/duplicates, redirects, and anchors.
- Web: symmetric collapse, asymmetric expansion, missing direction, no traffic
  duplication, filters, exact-time lane state, dense labels, and legacy copy.
- Playwright at 1440x900 and 390x844: no collisions or overflow, 44px targets,
  focus/bottom-sheet behavior, topology retention, and no console errors.

## Risks

- Tailscale status does not prove exact dual-send state. Every inferred state
  remains visibly and durably labeled with a versioned rule.
- Added virtual routes increase graph pressure. Endpoint positions remain
  authoritative and complete rendered footprints participate in collision
  checks.
- Old rows may lack enough provenance. They remain legacy/unknown rather than
  fabricated.
- Full-window path pagination can be large. Fetching is incremental and
  abortable; the UI never silently truncates.

## Current state

The API/storage slice is implemented. Collector snapshots now carry optional
fallback and evidence metadata, native and tsnet sources share the versioned
twelve-second monotonic inference tracker, and aggregation projects only
endpoint observations into two directions. Schema migration 6 persists and
legacy-backfills directions. The complete path-history API uses stable keyset
pagination and generated Go/TypeScript models are current.

The ready-for-review stack received fifteen actionable inline findings across
two review passes. Remediation remains dependency ordered. The API slice now
clears relay inference when a peer disappears, freezes pagination boundaries,
returns historical node references on every page, repairs schema-v5 ordering in
migration 6, records resolved relay endpoint/VNI changes, and rejects inferred
evidence without a versioned rule. The first Live and History review passes are
fixed; the second Live pass still requires one total-rate label per relationship
and a recent-path view in the inspector.

## Next step

Update PR #189, safely rebase PR #190 and PR #191 in order, then fix and test
the two remaining Live review findings before rerunning the complete gates.

## Verification

- Review-remediation tests cover a disappearing/reappearing peer, a moving
  wall clock during 900-event pagination, exclusion of events after the frozen
  window end, node references first encountered on a paginated page, v5 event
  ordering repair, resolved relay endpoint/VNI transitions, and mandatory
  inference rules at both validation boundaries.
- Focused exporter, adapter, domain, aggregation, store, HTTP, and app Go tests
  pass.
- Full `go test ./...` passes.
- Generated OpenAPI Go and TypeScript types are current.
- TypeScript check and all 73 current Vitest tests pass.

## Completion summary

Pending.
