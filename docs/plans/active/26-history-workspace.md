# History workspace implementation plan

Status: implemented; awaiting PR review
Issue: https://github.com/GhostFlying/tailpath/issues/26
Parent: https://github.com/GhostFlying/tailpath/issues/18
Last updated: 2026-08-24

## Context

The Live graph is the only current route. Issue #25 adds bounded node, edge-list,
and edge-detail history APIs. This issue implements the accepted History desktop
split workspace and mobile list/detail flow without increasing the Live route's
primary bundle or changing its default behavior.

## Accepted concepts

- Desktop: `docs/design/v0.2-history-desktop.png` (1586 x 992).
- Mobile list: `docs/design/v0.2-history-mobile-list.png` (853 x 1844).
- Mobile detail: `docs/design/v0.2-history-mobile-detail.png` (852 x 1846).
- Node anatomy: `docs/design/v0.2-node-anatomy.png` (1536 x 1024).

These images are production design specs. The node-anatomy concept is already
implemented by #23/#24 and remains the shared platform/telemetry vocabulary.

## Design system inventory

- Color lock: true white page and panels, cool light-gray separators/grid,
  near-black text, muted blue-gray labels, teal `#16877a`, DERP amber
  `#bd7b00`, Peer Relay magenta `#a4488e`, Unknown gray, and blue for the
  reverse traffic direction. No gradients, decorative cards, or tinted page
  background.
- Typography: existing Inter/system stack, zero letter spacing, 14-17px desktop
  content, 11-12px chrome labels, compact 600-700 headings, deliberate control
  sizing. Mobile scales spacing and container width, not font size by viewport.
- Container model: one app topbar, one unframed filter band, desktop vertical
  list/detail divider, flat list rows, open chart/timeline sections, and one
  bordered provenance table only. No nested cards.
- Icon family: existing Lucide outline icons for brand, search, chevrons, back,
  arrows, platform presentation, path symbols, observers, and clock warning.
  Direction remains explicit through up/down arrows and A-to-B/B-to-A labels.
- Motion: route/list selection and tooltip only; no decorative animation.
- Controls: desktop segmented window control, node autocomplete input, path
  select, list rows, pagination command; mobile native selects for compact
  window/path controls and a full-width search input.

## Copy and information architecture

- Topbar copy is limited to `Tailpath`, `Live`, `History`, and the current
  History HTTP state (`Connecting`, `Reachable`, or `Unavailable`).
- Filter copy is limited to fixed windows, `Find node`, and path choices.
- List headings are `Connections`, `Last traffic`, and `Total`; rows show the
  canonical endpoint labels, path label(s), last traffic, directional totals,
  and telemetry warning when present.
- Detail copy is limited to endpoint title, `Last path`, `Last traffic`,
  `Traffic over time`, direction labels, `Path timeline`, observer count,
  `Observed by`, table fields, and bounded loading/error/empty states.
- Desktop `/history` selects the first returned edge when no detail ID is in the
  URL. `/history/edges/:edgeId` deep-links selection in the same split view.
  Mobile `/history` is list-only and selection navigates to a full-screen detail
  route whose back control and browser back return to the preserved list URL.

## Component architecture

- `AppRouter` owns `BrowserRouter` and lazy route boundaries.
- `LiveWorkspace` is the existing Live app moved without functional changes.
- `HistoryWorkspace` owns URL query state, parallel node/list requests, deferred
  autocomplete search, pagination, and desktop/mobile composition.
- `HistoryFilters`, `HistoryEdgeList`, and memoized `HistoryEdgeRow` own chrome
  and list rendering. An ID Map supplies node lookup; rows use
  `content-visibility: auto`.
- `HistoryDetail` owns detail fetch states and composes memoized
  `DirectionalTrafficChart`, `PathTimeline`, and `ProvenanceTable`.
- `historyUrl` parses/serializes `window`, `nodeId`, `path`, and `cursor` while
  preserving browser navigation. Invalid URL values fall back to `24h`/all
  without emitting invalid API requests.
- `historyMath` converts byte buckets to non-negative rates, builds symmetric
  SVG geometry, derives path segments from anchor/events, and indexes
  provenance without adding a chart dependency.

## Data and performance decisions

- History route code is loaded by `React.lazy`; `react-router-dom` is the only
  new runtime dependency. Live components and Cytoscape stay outside the
  History chunk, and History code never imports the Live graph.
- Node and edge-list requests begin together when window changes. Detail begins
  as soon as the selected/deep-linked edge ID is known. AbortController cancels
  superseded requests; stale responses cannot replace current URL state.
- Node search uses `useDeferredValue` against the at-most-250 node response.
  Edge rows are memoized and backed by maps/sets instead of repeated scans.
- Traffic SVG has a stable viewBox and at most 200 points. A-to-B renders above
  zero, B-to-A below; buckets use their real timestamp extents, missing samples
  remain gaps, and tooltip values remain real non-negative rates.
- Path timeline has at most 501 states including anchor. Selection is local UI
  state and reveals provenance. Color is always paired with path text and a
  distinct symbol.
- Pagination navigates keyset pages through the URL. Changing window/node/path
  clears cursor and list; browser back returns to the prior page. URL holds
  filter state and selected edge, not fetched data.

## Steps

- [x] Save accepted concepts and add router/lazy route scaffolding.
- [x] Add typed history API clients, URL state, loading/error/empty behavior,
  node autocomplete, path-seen filter, and pagination.
- [x] Implement accepted desktop split and mobile list/detail layouts.
- [x] Implement directional SVG traffic, tooltip, path timeline selection,
  provenance table, and clock-skew warnings.
- [x] Add fixture history depth sufficient for charts/transitions.
- [x] Add unit and Playwright coverage for deep links, URL filters, back,
  pagination, loading/error/empty, chart direction, and responsive layouts.
- [x] Run bundle inspection and verify History is lazy without increasing the
  Live entry chunk materially.
- [x] Capture native-size desktop/mobile screenshots, compare all accepted
  concepts with `view_image`, and complete the fidelity ledger.

## Fidelity ledger

| Comparison point | Concept evidence | Render evidence | Status |
| --- | --- | --- | --- |
| White app shell/topbar and teal active route | All accepted screens | Desktop and mobile screenshots; mobile reachable dot restored | Match |
| Desktop filter band and split list/detail geometry | Desktop concept | 1586 x 992 Playwright screenshot | Match |
| Mobile list-only rows and compact filters | Mobile list concept | 1118 x 2420 Pixel 7 screenshot | Match |
| Mobile full-screen detail and browser/back behavior | Mobile detail concept | 1118 x 2420 screenshot plus browser/back assertions | Match |
| Symmetric traffic chart with direction labels | Desktop/mobile detail | Non-empty mirrored SVG paths; non-negative tooltip rates | Match |
| Path symbols/text, selection, provenance, clock warning | Desktop/mobile detail | Selectable Direct/DERP timeline and amber provenance warning | Match |
| Typography, separators, flat container model | All accepted screens | Latest screenshots inspected at original resolution | Match |

## Intentional concept differences

- Platform glyphs use the shared Lucide-derived device vocabulary from #23
  rather than vendor trademarks shown in the early concepts.
- Rows say `2 paths seen` when the API returns multiple path kinds instead of
  presenting the last path as if it were the only path in the selected window.
- Screenshot fixture totals/timestamps and observer counts are deterministic
  test values, not copied visual placeholders from the concepts.

## Tests

- Vitest covers URL normalization, rate/geometry math, timeline construction,
  path labels/symbols, and node filtering.
- Playwright intercepts history APIs for deterministic full, loading, error,
  empty, pagination, path filter, and clock-skew states. It asserts routes and
  browser/mobile back behavior, chart directions, timeline selection, no
  overlap/overflow, and no console errors.
- A separate Playwright case opens seeded history through the real fixture API
  on desktop and mobile. Existing Live desktop/mobile suites pass to prove
  routing/lazy loading did not regress Live; scale remains a dedicated fixture
  mode.
- Browser/IAB was unavailable. Regular Playwright Chromium was the approved
  fallback, using the pinned local executable documented by the repository.
- Production build output keeps History in a dedicated 6.15 KiB gzip chunk;
  the shared entry is 73.33 KiB gzip and Live remains isolated at 151.05 KiB.

## Risks

- Importing History through a shared barrel can pull it into the Live entry
  chunk. Route modules use direct imports and production bundle inspection.
- Browser back can lose list filters if selection uses component-only state.
  Selection and filters are URL state; mobile detail returns to the exact list
  search string.
- Mirroring signed rates in tooltip text would expose negative values. Geometry
  applies sign only to SVG coordinates; labels always format absolute rates.
- A path anchor is state at the window start, not a transition inside it. The
  timeline visually starts at `from` and does not claim the anchor timestamp as
  an in-window change.

## Current state

The workspace, real fixture history, URL behavior, responsive layouts, visual
comparison, bundle inspection, unit tests, and full Live/History Playwright
suite are complete. Concurrent browser validation also exposed and fixed the
parent store's in-memory SQLite lifetime bug and canceled-request log noise in
#25 before this branch was rebased.

The shared topbar now requires an explicit workspace signal: History reflects
its required HTTP requests instead of inheriting Live's SSE label.

## Next step

Push the rebased branch, refresh Draft PR #36, and run the repository-wide
generated-file, race, build, and browser gates before marking it ready.
