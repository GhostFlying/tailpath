# Exact History last-traffic implementation plan

Status: completed
Issue: https://github.com/GhostFlying/tailpath/issues/63
Last updated: 2026-08-25

## Context

History edge summaries expose the exact server-received `lastTrafficAt`, but
edge detail derives its label from the final aggregated bucket start. The
result can be early by one complete 12-minute or one-hour display bucket.

## Decisions

- Add optional `lastTrafficAt` to `EdgeHistory`. It is present when the
  selected window contains traffic and absent for a known edge with an empty
  window.
- Source the value from canonical history-edge metadata, never from a chart
  bucket. Canonical aliases use the latest exact timestamp across their
  physical edges.
- Keep bucket timestamps, resolutions, point bounds, and retention unchanged.
- Render the detail value as a semantic `time` element and preserve `No
  traffic` for an empty window.

## Steps

- [x] Extend OpenAPI and regenerate Go and TypeScript models.
- [x] Populate exact detail metadata and cover 24-hour, seven-day, alias, and
  known-empty queries.
- [x] Use the exact field in History detail and assert list/detail agreement.
- [x] Run focused checks, the complete `make check` stages, and archive this
  plan on completion.

## Verification

- Generated Go and TypeScript models reproduce without a diff.
- Go formatting, vet, and the complete Go test suite pass under Go 1.26.
- TypeScript, Prettier, Vitest (46 tests), and the production web build pass
  with Node 24 and pnpm 10.15.0.
- Playwright passes 13 applicable desktop/mobile tests; seven intentionally
  gated tests remain skipped in the default fixture run.
- Visual inspection confirms the exact Last traffic value agrees between the
  History list and detail on desktop and mobile.

## Current state

`EdgeHistory.lastTrafficAt` now carries exact canonical history metadata when
the selected window contains traffic. The web detail renders that value and
does not derive user-visible recency from an aggregate bucket coordinate.

## Next step

Open the implementation PR for review and close issue #63 when it merges.
