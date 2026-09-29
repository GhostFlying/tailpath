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
   The appended `directions_tracked` marker distinguishes an explicit empty
   withdrawal from an unprojectable legacy event.
6. A separate keyset-paginated path-history endpoint removes the existing 500
   event correctness cap while the old embedded detail field remains.
7. Missing directions are unknown. They do not trigger asymmetric expansion.

## Interfaces

Observer peers add optional `fallbackPath`, `pathEvidence`, and
`pathInferenceRule`. Topology edges and path events add `directions[]`; path
events also expose `directionsTracked` for empty-state interpretation.
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

The API/storage, Live, and History slices are implemented. Collector snapshots now carry
optional fallback and evidence metadata, native and tsnet sources share the
versioned twelve-second monotonic inference tracker, and aggregation projects
only endpoint observations into two directions. Schema migration 6 persists
and legacy-backfills directions; append-only migration 7 marks future events
as directionally tracked without reclassifying ambiguous existing empty rows.
The complete path-history API uses stable keyset pagination and generated
Go/TypeScript models are current.

Live now collapses equivalent directions, expands asymmetric primary/fallback
states into destination-arrow route chains, and renders DERP fallback as an
unmetered dashed route. The inspector fixes two direction slots, leaves a
missing direction unknown, labels evidence, and separates endpoint path
evidence from relay-session identity evidence. Legacy edges without directions
retain their compatibility candidate view.

History now consumes the keyset-paginated path endpoint to completion and
reports incremental progress. Directional records render on a shared
left-to-right axis with two primary lanes, two DERP fallback sublanes, a shared
traffic cursor, and an exact-time desktop table or mobile bottom sheet. Labels
are omitted below the 56px budget, dense fallback windows are summarized, and
every event remains available in a 44px-target detail index. Pre-directional
records retain the legacy newest-first view and explicit combined-evidence
copy.

The ready-for-review stack received fifty-six actionable inline findings, all now
addressed.
The API slice now clears relay inference when a peer disappears, freezes
pagination boundaries at an explicit detail timestamp, returns historical node
references on every page, preserves the released schema-v5 migration boundary,
records resolved relay endpoint/VNI changes, and rejects inferred evidence
without a versioned rule. It also clears directions when endpoint evidence is
withdrawn, includes fallback kinds in History filters, rejects fresh legacy
evidence, preserves Direct address changes while ignoring port churn, and uses
indexed fixed-width nanosecond timestamps scoped to path events for exact
History boundaries without changing other persisted timestamp formats. The
API also archives an explicit empty-direction transition when endpoint evidence
is withdrawn and uses full timestamp precision when retaining path anchors.
Path-history cursors carry the canonical edge ID, accepting aliases of that
edge while rejecting reuse against another edge. A failed native or tsnet
status poll resets relay inference continuity, so the next DERP sample cannot
inherit relay evidence from before the observation gap. Each path cursor also
retains the first page's row-ID high-water mark, excluding in-window events
that commit after the selected snapshot. The optional
`directional-path-evidence` capability lets a new exporter omit all three new
peer fields when reporting to an older strict-decoding protocol-v1 server. The
Live slice now derives arrows from each observed direction's own rate,
distinguishes partial evidence, suppresses recent arrows, exposes relay
resolution, renders one total rate label per logical relationship, retains the
recent-path inspector, treats complete relay identity as the shared Live and
recent-event comparison key, separates reciprocal route geometry, and
describes observed and inferred fallback evidence without conflating them.
Obstacle routing coordinates reciprocal curves on the same signed side instead
of letting independently selected offsets cancel, and logical edge selection
is reapplied whenever collapsed and expanded route IDs replace one another.
Directional arrows require actual flow, and per-edge DERP marker identities
cannot join unrelated routes, while partial Recent paths retain their fallback
detail. Direct asymmetric routes also honor the single relationship-label
assignment. The default reciprocal curves are sampled into the same rendered
geometry used for obstacle and route-crossing checks, and DERP fallback is
classified under the Path legend rather than traffic Activity.

History preserves unknown directions in summaries, renders complete
per-direction evidence and relay resolution metadata, keeps compatibility
events visible while pagination runs, fixes the first page to the detail
response's absolute end, reports paging through its readiness state, adapts the
evidence table between 621px and 1100px, and timestamps relay-session evidence
with an explicit collector clock warning when needed. Its relationship summary
and exact-time snapshot share the same complete relay identity key, preserving
endpoint, VNI, and resolution asymmetry. Timeline selection uses a stable event
identity across pagination replacement, and a missing direction keeps both its
primary and fallback cells unknown. Dense timelines are coalesced into
elapsed-time pixel bins rather than event-count chunks. Uniform bins preserve
the actual state while mixed bins are explicitly rendered as dense changes, so
a burst cannot stretch its latest state across an earlier long-lived path. The
complete event index is mounted in accessible 100-row pages, keeping every
retained state reachable without unbounded DOM growth. Empty-direction events
after directional evidence are rendered as withdrawn/unknown rather than
falling back to the compatibility path, and stable selection keys retain only
a compact digest instead of the complete event JSON. The legacy timeline uses
the same 100-row paging bound, and its observer support comparison mirrors the
server compatibility key so endpoint or VNI changes for an already resolved
StableNodeID do not incorrectly become contradictory evidence.
Event-index paging is stored as an offset from the newest state, so prepending
older pages keeps a selected, focused event on the same visible page instead of
jumping the index backward.

## Next step

Run the complete repository gate, update the rebased PR stack, then reply to and
resolve the two newly addressed API review threads before requesting a fresh
review.

## Verification

- Review-remediation tests cover a disappearing/reappearing peer, a moving
  wall clock during 900-event pagination, exclusion of events after the frozen
  window end, node references first encountered on a paginated page, the
  irrecoverable schema-v5 deduplication boundary, resolved relay endpoint/VNI
  transitions, withdrawn directions, primary/fallback History filtering, and
  rejection of inferred-without-rule or fresh legacy evidence at both report
  boundaries, Direct address-versus-port changes, and exact nanosecond window
  bounds across migrated RFC3339Nano timestamps. Cursor coverage accepts a
  canonical alias, rejects cross-edge reuse, and excludes an in-window event
  committed after the first page. Native and tsnet adapter tests
  prove failed polls break inference continuity.
- Exporter negotiation tests verify current servers receive directional fields
  while older protocol-v1 servers receive none of the three new JSON fields.
- Live review-remediation tests cover reverse-only collapsed traffic, partial
  direction copy and flow rates, exact relay resolution metadata, fallback
  evidence wording, and desktop/mobile rendering.
- Focused exporter, adapter, domain, aggregation, store, HTTP, and app Go tests
  pass.
- Full `go test ./...` passes.
- Generated OpenAPI Go and TypeScript types are current.
- TypeScript check and all 100 current Vitest tests pass.
- Focused directional Live Playwright coverage passes on desktop Chromium at
  1440x900 and mobile Chromium at Pixel 7 dimensions, including no horizontal
  overflow, single-counted traffic, and no console errors.
- The full Live browser matrix passes on desktop and mobile: 68 passed and 30
  intentionally skipped, including one total-rate label, the restored
  recent-path inspector, distinct VNI identities, and visibly separated
  reciprocal relay lanes. Recent path events also use the complete relay
  identity key.
- Focused obstacle and relay UI browser coverage passes: 23 passed and one
  intentionally skipped across desktop and mobile, including reciprocal
  obstacle routes and selection across route-ID replacement.
- Directional History Playwright coverage passes on desktop and mobile,
  including focus-restoring bottom-sheet interaction, 56px label disclosure,
  horizontal-overflow checks, shared traffic selection, and exact-time state.
- A two-page 900-event browser fixture renders all 900 events and exposes the
  500-event intermediate loading state without truncation or duplication. It
  also selects an embedded state before pagination completes and verifies that
  the same state remains visible and selected after older events are prepended.
- A 900-event legacy browser fixture pages the timeline in 100-state windows,
  keeps the mounted list bounded, and preserves access to every state.
- An uneven-duration dense fixture preserves a 23-hour Direct state and marks
  only the final one-hour burst as mixed at a 24-bin render budget.
- The complete repository gate passed: generated-file consistency, shell
  harnesses, formatting, `go vet`, all Go tests, TypeScript, 100 Vitest tests,
  the production Web build, and the browser matrix.
- The final review-remediated Chromium browser matrix passed with CI
  concurrency: 73 passed and 33 intentionally skipped across desktop and mobile
  projects, including the absolute paging boundary, incomplete readiness state,
  intermediate desktop width, complete relay observation timestamps, legacy
  900-event paging, and 4293/8 asymmetry in both the summary and exact-time
  snapshot.
- The layout/response/obstacle Chromium gate passed with the same diagnostics
  flags as CI: 23 passed and one desktop-only 320px duplicate intentionally
  skipped. WebKit remains assigned to the hosted layout workflow because it is
  not installed in the local browser cache.
- The main-targeted History replay persists and exposes `directionsTracked`,
  so an empty directional anchor remains `Unknown / No fresh observation`
  without misclassifying an unprojectable migrated event. Focused storage,
  API, timeline, and 900-event browser regressions cover the window-boundary,
  migration, and legacy paging cases.
- Generated-file consistency, all Go tests, 102 Vitest tests, the production
  build, and both 900-event browser regressions pass after migration 7. The
  complete local browser matrix passed 72 tests and hit one unrelated mobile
  retry-button detach race; that exact test then passed three consecutive
  parallel reruns.
- Final Live and History desktop/mobile screenshots were compared against all
  three accepted concepts with `view_image`; the implemented hierarchy and
  interaction match while code-native copy replaces concept-only labels.
- The Browser plugin is unavailable in this environment; repository Playwright
  is the recorded browser-validation fallback.

## Completion summary

The three implementation slices are complete: durable directional evidence and
pagination, asymmetric Live rendering, and paginated directional History. The
compatibility API remains available, old evidence is honestly labeled, DERP
fallback never duplicates traffic totals, and no active network observation
was introduced. Release and exact-digest deployment remain gated on human
review and ordered rebase-merges.
