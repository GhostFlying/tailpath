# Bounded seven-day history implementation plan

Status: active
Issue: https://github.com/GhostFlying/tailpath/issues/25
Parent: https://github.com/GhostFlying/tailpath/issues/18
Last updated: 2026-08-24

## Context

Tailpath currently writes per-observer ten-second traffic rows and path events
directly into one draft SQLite schema. History has one unbounded edge-detail
query, no numbered migrations, no canonical redirect record, and deletes all
time series at one retention cutoff. v0.2 needs bounded fixed-window queries and
seven days of logical history without summing bilateral observations.

## Goals

- Migrate existing dogfood databases in place using `PRAGMA user_version`.
- Retain per-observer 10-second raw traffic for one hour, deduplicated logical
  one-minute traffic for 48 hours, and logical one-hour traffic for seven days.
- Preserve the existing directional preference rule independently in every raw
  ten-second bucket before summing logical buckets.
- Persist canonical node redirects and resolve them before grouping edge and
  node history.
- Preserve one pre-window path anchor for edges with retained traffic.
- Add bounded node search, edge list, and edge detail store/API contracts for
  the fixed 15m, 1h, 6h, 24h, and 7d windows.

## Non-goals

- Arbitrary time ranges, custom resolutions, export, backup/restore, or a
  Prometheus-compatible metrics API.
- Control-plane inventory or nodes that have never appeared in business
  traffic.
- History UI; issue #26 consumes the API introduced here.

## Decisions

- Migration 1 adopts the current draft schema and compatibility columns;
  migration 2 appends history metadata, redirect, rollup, and maintenance-state
  tables plus query indexes. Every successful migration advances
  `PRAGMA user_version` in its transaction. Released migration functions are
  append-only.
- `history_edges` records every logical edge that has accepted traffic, its
  canonical endpoints, and first/last traffic receive time. Rows outlive series
  retention so a known edge with no traffic in the selected window returns an
  empty 200 response; an ID never recorded is 404.
- Aggregator runtime state records canonical redirects. On merge, existing
  redirect targets are path-compressed to the survivor. Full node metadata and
  redirects are written atomically with checkpoint-bearing reports (at most
  once per second), and are refreshed after startup replay.
- Rollup maintenance processes only ended minute/hour buckets. The minute
  rollup first applies endpoint-observer preference for each edge/direction in
  each ten-second bucket, falling back to third-party maximum, then sums those
  logical values. Hour rollup sums already-deduplicated minute rows.
- Maintenance cursors make ended-bucket work idempotent. Upserts replace a
  completed logical bucket, so interrupted maintenance can safely retry.
- Raw traffic expires after one hour, minute rollups after 48 hours, and hour
  rollups after seven days. Retention is based only on server receive time.
- Path maintenance keeps the latest event before the seven-day cutoff only when
  the canonical edge still has retained traffic; all older events and anchors
  for fully expired edges are removed.
- Fixed windows map to resolutions as follows: 15m/10s, 1h/30s, 6h/3m,
  24h/12m, 7d/1h. Store queries re-bucket the retained layer and never return
  more than 200 traffic points or 500 path transitions.
- Edge list pagination is stable keyset pagination by descending last traffic,
  then canonical edge ID. The opaque cursor encodes those two fields. `path`
  means an event or anchor for that path exists in the window, not merely that
  the last event matches.
- Canonical node references use the opaque Tailpath ID plus current display
  identity. Redirects are resolved before edge grouping so pre/post-merge data
  produces one history edge.

## Interfaces

- `HistoryWindow` validates fixed window strings and exposes duration and
  resolution.
- `GET /api/v1/history/nodes?window=` returns at most 250 canonical node
  references that occur on traffic edges in the window.
- `GET /api/v1/history/edges?window=&nodeId=&path=&cursor=&limit=` returns a
  descending page with an optional next cursor; limit defaults to 50 and caps
  at 100.
- `GET /api/v1/history/edges/{edgeId}?window=` returns canonical source/target,
  window bounds, bucket duration, two directional series, the path anchor,
  bounded path events, and truncation flags.
- Unknown edge IDs return 404. A recorded edge without traffic in the selected
  window returns 200 with empty traffic/events.

## Steps

- [x] Introduce append-only numbered migrations and migration compatibility
  tests for current dogfood databases.
- [x] Persist canonical node metadata, redirects, and traffic edge metadata.
- [x] Implement idempotent ended-bucket minute/hour rollups and tier retention.
- [x] Preserve path anchors and add retention/query indexes.
- [x] Define the OpenAPI history contracts and regenerate Go/TypeScript types.
- [x] Implement bounded store queries, redirect grouping, filters, and cursor.
- [x] Implement HTTP validation/status semantics and API tests.
- [x] Run migration, retention, scale-size, generated-file, race, and full
  repository verification.

## Tests

- Open/migrate tests assert user versions, draft-column backfills, data
  preservation, and idempotent reopen.
- Rollup tests cover bilateral endpoint observations, third-party fallback,
  directional independence, ended-bucket boundaries, retry, and all retention
  cutoffs.
- Redirect tests merge a placeholder into a stable node and assert one logical
  edge across raw, rollup, list, nodes, and detail queries.
- API tests cover every window/resolution, path-seen and node filters, cursor
  boundaries, default/max/invalid limit, unknown edge 404, known empty 200, and
  200/500 caps.
- A deterministic seven-day 1,000-edge database fixture records final file size
  and enforces the 2GB target in the manual scale workflow.

## Risks

- Summing observers before applying directional precedence double-counts the
  same traffic. Deduplication must occur per edge, direction, and raw bucket.
- Deleting raw rows before their ended minute is committed creates permanent
  gaps. Rollup and deletion remain one maintenance transaction.
- Redirect cycles or chains can split history or loop queries. Writes reject
  self redirects and compress chains; reads retain a visited set and resolve to
  a deterministic terminal ID.
- Path events can exceed the response cap during flapping. Queries fetch the
  anchor separately, return the newest 500 in chronological order, and set an
  explicit truncation flag.

## Verification record

- Migration tests preserve current draft databases, backfill legacy columns,
  advance to user version 2, reopen idempotently, and reject future versions.
- Store tests cover bilateral directional preference, third-party fallback,
  idempotent rollup, ended buckets, all three retention boundaries, path
  anchors, redirects with direction reversal, filters, cursor pagination,
  known-empty detail, and transition caps.
- API/generated Go types compile; generated TypeScript passes typecheck and
  formatting. HTTP tests cover required windows, invalid path/limit/cursor,
  list/nodes/detail success, known-empty 200, and unknown 404.
- The manual 250-node/1,000-edge steady seven-day fixture wrote 3,768,000
  traffic rows and produced a 707,936,256-byte database in 67.7 seconds, below
  the 2 GiB target.
- Generated Go and TypeScript hashes were unchanged after regeneration. Full Go
  vet/test/race/build, Web typecheck/format/39 unit tests/build, and normal
  desktop/mobile Playwright passed. The existing 500 kB Live bundle warning
  remains; #26 owns History route lazy loading and bundle separation.

## Current state

Migration, storage, aggregation metadata, query, API, manual size gate, and
full repository verification are complete.

## Next step

Push the verified branch and request review of the stacked Draft PR.
