# Stable live-graph layout implementation plan

Status: active
Issue: https://github.com/GhostFlying/tailpath/issues/24
Parent: https://github.com/GhostFlying/tailpath/issues/18
Last updated: 2026-08-24

## Context

The live graph currently reruns whole-graph COSE and fits the viewport whenever
the visible element signature changes. Path filters, Show recent, edge aging,
and reconnects therefore move nodes or reset pan/zoom. A cold 250-node/1,000-edge
fixture also spends about 19 seconds laying out from random positions on every
refresh. This issue makes canonical-node coordinates durable browser state and
turns layout into an explicit, incremental operation.

## Goals

- Persist versioned canonical-node positions in localStorage.
- Ignore malformed, oversized, stale, or non-finite cache data and keep at most
  2,000 entries seen within 30 days.
- Preserve node coordinates and viewport across edge rate/path/state changes,
  SSE reconnect, filters, recent visibility, aging, and refresh.
- Restore cached nodes exactly and place only genuinely new canonical nodes near
  known-neighbor centroids before an incremental layout.
- Derive DERP and Unknown marker positions from current edge endpoints and never
  persist virtual markers.
- Add icon-only Fit and Relayout controls with accessible names and tooltips.
- Keep ordinary device nodes exactly 52 by 52 pixels.

## Non-goals

- Synchronize coordinates between browsers or users.
- Persist pan/zoom, selected items, filters, or virtual path markers.
- Replace Cytoscape or COSE, add server-side layout, or optimize History charts.
- Guarantee identical coordinates after an explicit Relayout.

## Decisions

- Storage key is `tailpath.graph-layout.v1`; payload version is `1` and contains
  canonical ID, finite model coordinates, and last-seen epoch milliseconds.
- Reads reject the whole payload when shape/version is invalid or it contains
  more than 2,000 entries. Individual invalid/stale entries are discarded.
- Writes merge current visible positions with retained entries, refresh
  last-seen for topology nodes, prune at 30 days, sort newest first, and cap at
  2,000. Storage quota/security failures are non-fatal.
- Graph elements mark real topology nodes as persistable. DERP and Unknown
  virtual markers do not carry that marker; a real Peer Relay remains canonical
  and persistable despite its path-specific appearance.
- Initial render restores cache first. Unknown canonical nodes are seeded at a
  known-neighbor centroid plus a deterministic small offset; remaining nodes use
  a deterministic grid. Cached/existing nodes are locked while COSE lays out only
  new nodes with fit disabled. Automatic COSE is bounded to graphs with at most
  100 canonical nodes; larger graphs keep the deterministic seeded positions so
  the browser cannot be blocked by an unbounded initial layout.
- After structure updates, virtual marker coordinates are recomputed from the
  average positions of their current neighboring endpoints. Edge-only changes
  do not invoke COSE or edge-clearance mutation.
- Fit changes only pan/zoom. Relayout clears the cache, unlocks all real nodes,
  runs randomized full COSE, derives markers, fits, and saves the result.
- Drag completion persists the moved canonical node without running layout.

## Interfaces

- `readLayoutCache`, `writeLayoutCache`, and `clearLayoutCache` isolate browser
  storage validation and retention.
- Graph element data adds `persistable: true` only for canonical topology nodes.
- `TopologyGraph` renders Fit and Relayout icon buttons over the graph stage.
- The canvas exposes bounded position fingerprints and square-node status as
  test diagnostics; they are not API contracts.

## Steps

- [x] Implement versioned bounded layout-cache helpers and unit tests.
- [x] Mark canonical graph elements and seed cached/new node coordinates.
- [x] Replace signature-driven full layout with initial/incremental layout and
  derived marker placement while preserving viewport.
- [x] Add drag persistence plus Fit and Relayout controls.
- [x] Extend Playwright for filter, recent, reconnect, refresh, explicit
  relayout, square nodes, and desktop/mobile behavior.
- [x] Run normal and scale verification, inspect screenshots, and record results.

## Tests

- Vitest covers malformed JSON, wrong version, over 2,000 entries, stale data,
  finite-coordinate validation, pruning, cap ordering, storage failures, and
  clear behavior.
- Graph unit tests prove only canonical nodes are persistable and virtual marker
  identity remains derived.
- Playwright records canonical positions and viewport, then exercises path
  filters, Show recent, topology invalidation/reconnect, and refresh without
  movement. Fit changes viewport only; Relayout changes coordinates and stores
  the new result.
- Desktop and mobile runs retain strict square-node assertions. The scale run
  reloads from cache to demonstrate a faster stable second render.

## Risks

- Running COSE over the full graph without locking existing nodes would preserve
  neither coordinates nor user context. Every incremental layout must use a
  finally-style unlock path.
- Removing filtered nodes can lose positions unless the cache is updated before
  removal and consulted when they return.
- Shared DERP nodes have several endpoint pairs; their derived position must use
  all current neighbors rather than whichever edge is visited last.
- localStorage can throw in privacy/quota modes. Graph rendering must remain
  fully functional with an in-memory empty cache.

## Verification record

- `pnpm --dir web test`: 39 tests passed.
- `pnpm --dir web check`: TypeScript and formatting passed.
- Normal Playwright fixture: desktop and mobile suites passed, including stable
  coordinates/viewport across SSE, filters, Show recent, refresh, Fit, and
  Relayout.
- Scale Playwright fixture: 250 topology nodes, 1,000 logical edges, 505 rendered
  nodes, all path/state variants, and nine clock-skew observers rendered without
  console errors. Desktop `data-ready` passed the 5-second assertion; a cached
  reload performed zero automatic layout runs and restored exact coordinates.
- The desktop scale screenshot was visually inspected. The dense all-path view
  remains intentionally filter-driven, while platform icons, clock warnings,
  path colors, controls, and the legend remain visible.

## Current state

Implementation and issue-level verification are complete. The remaining work is
full repository validation and review of the stacked Draft PR.

## Next step

Run `make check`, push the issue branch, and open the Draft PR against #23.
