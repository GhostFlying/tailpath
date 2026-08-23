# Incremental runtime checkpoints implementation plan

Status: active
Issue: https://github.com/GhostFlying/tailpath/issues/21
Parent: https://github.com/GhostFlying/tailpath/issues/18
Last updated: 2026-08-23

## Context

The #20 baseline takes about 23 seconds to accept 750 reports because every
submission JSON-serializes the full runtime graph for cloning, persistence, and
replacement. Runtime refresh bursts can also cause overlapping browser fetches.
This issue removes those per-report whole-state costs without weakening the
transactional publish boundary.

## Goals

- Use typed deep copies for candidate aggregation state and transfer candidate
  ownership after a successful database commit.
- Persist every accepted report, traffic contribution, and path transition in
  one transaction while checkpointing runtime state at most once per second.
- Restore a checkpoint and replay only later committed report rows.
- Delete only checkpoint-covered raw reports during minute maintenance.
- Bound topology SSE invalidation to four events per second per client and
  collapse browser refresh bursts into one in-flight request plus one follow-up.

## Non-goals

- Change observer protocol, traffic rollup retention, history API, or collector
  retry behavior.
- Enforce the final 125 reports/s release gate; #28 owns sustained RC testing.
- Relax the rule that storage failure cannot publish or advance runtime state.

## Decisions

- The existing `reports` implicit SQLite rowid is the commit cursor. A runtime
  checkpoint stores the greatest rowid represented by its payload.
- A missing or legacy checkpoint cursor is zero. Replaying retained rows is
  safe because reporter sequence/report-ID handling is idempotent.
- The first changed report checkpoints immediately. Later reports checkpoint
  when server receive time is at least one second beyond the prior checkpoint;
  receive-time rollback also forces a checkpoint rather than delaying forever.
- Report, traffic, transition, and optional checkpoint writes share one SQLite
  transaction. A nil checkpoint payload means this report is journal-only.
- Maintenance is explicit application work on a one-minute ticker. It deletes
  raw rows no newer than the committed checkpoint and applies existing history
  retention using server time.
- SSE uses a 250 ms trailing window. Browser requests use a small generic
  single-flight runner, not abort-and-replace, so bursts yield at most one
  pending follow-up and no resource-error noise.

## Interfaces

- Store checkpoint restore returns payload, last report rowid, and update time.
- Stored reports include rowid and can be restored after an exclusive cursor.
- `SQLite.Maintain(ctx, now)` performs checkpoint-aware raw cleanup and current
  retention cleanup.
- `App.RunMaintenance(ctx)` owns the minute ticker and is started by the server.
- Aggregator cloning and replacement preserve subscribers and transfer only
  runtime state ownership.

## Steps

- [x] Implement typed runtime-state deep copy and ownership transfer tests.
- [x] Add checkpoint cursor schema compatibility and incremental record APIs.
- [x] Restore checkpoint plus later reports and cover crash/replay boundaries.
- [x] Add minute maintenance and checkpoint-covered raw-report cleanup tests.
- [x] Coalesce SSE invalidations and browser topology refreshes.
- [x] Rerun scale baseline and document the before/after measurements.

## Tests

- Deep-copy tests mutate every nested map, pointer, set, identity IP slice, and
  edge observation independently.
- Store tests cover first checkpoint, journal-only reports, replay cursor,
  legacy checkpoint migration, and maintenance safety.
- App tests cover storage failure, restart between checkpoints, restart after
  checkpoint, and identical topology after replay.
- HTTP tests prove an invalidation burst emits no more than four topology events
  per second at the configured test interval.
- Vitest proves max concurrency one and exactly one follow-up for a burst.
- The deterministic 250/1,000 app and browser baselines run again without lost
  topology or Chromium resource errors.

## Risks

- A shallow copy can let failed candidate mutation leak into committed state.
  Deep-copy coverage must include every runtime-state field.
- Advancing the checkpoint cursor outside the report transaction can lose
  replay data. Payload and cursor must update atomically with the represented
  report row.
- Deleting uncheckpointed rows would make crash recovery incomplete. Cleanup
  must read the committed cursor inside its transaction.
- Coalescing must retain a final event after a burst and must exit promptly when
  the SSE request or React effect is cancelled.

## Current state

Implementation and local verification are complete. The branch is ready for a
stacked Draft PR based on the #20 scale-harness branch.

## Next step

Run GitHub Actions on the stacked PR, then retarget it after the dependency PRs
are rebase-merged.

## Verification

- Typed deep-copy and ownership-transfer tests cover every nested runtime-state
  collection while preserving existing subscribers.
- Store tests pass for checkpoint cursors, journal-only reports, legacy cursor
  migration, replay queries, and checkpoint-safe maintenance cleanup.
- App tests pass for crash-window replay, new-checkpoint generation, receive
  time rollback, storage-failure atomicity, and recovery after all covered raw
  reports are removed.
- `go test ./...` and `go vet ./...` pass with Go 1.26.6. The scale app test
  improved from about 23.0 seconds and 5.37 MB to 1.88 seconds and 4.04 MB.
- Web typecheck/Prettier, 22 Vitest tests, and production build pass with Node
  24.19.0 and pnpm 10.15.0.
- Default Playwright passes four desktop/mobile tests. Scale Playwright passes
  desktop and Pixel 7 with 1,000 logical edges, 666 active/334 recent edges,
  nine skewed observers, and zero console errors.
- Browser plugin was unavailable, so repository Playwright Chromium was the
  explicit fallback. Scale screenshots were inspected as nonblank.

## Completion summary

Runtime state now uses typed candidates and bounded checkpoints, restart replay
uses a durable report cursor, maintenance deletes only checkpoint-covered raw
reports, and invalidation bursts are bounded end to end.
