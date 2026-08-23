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

- [ ] Implement typed runtime-state deep copy and ownership transfer tests.
- [ ] Add checkpoint cursor schema compatibility and incremental record APIs.
- [ ] Restore checkpoint plus later reports and cover crash/replay boundaries.
- [ ] Add minute maintenance and checkpoint-covered raw-report cleanup tests.
- [ ] Coalesce SSE invalidations and browser topology refreshes.
- [ ] Rerun scale baseline and document the before/after measurements.

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

The implementation boundary and current call chains are documented. No code has
been changed yet.

## Next step

Implement typed aggregator state cloning and exhaustive independence tests
before changing persistence.

## Verification

Pending.

## Completion summary

Pending.
