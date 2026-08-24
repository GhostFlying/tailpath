# Canonical identity replay remediation plan

Status: complete
Issue: https://github.com/GhostFlying/tailpath/issues/44
Parent: https://github.com/GhostFlying/tailpath/issues/40
Last updated: 2026-08-24

## Context

Runtime checkpoints are normally bounded to one per second. A journal-only
report can currently allocate a random canonical node ID, persist traffic under
that ID, and then be replayed after a crash with a different random ID. The
stored history and restored runtime topology then refer to different nodes.

## Decision

- Ordinary runtime changes retain the one-second checkpoint cadence.
- Any accepted report that allocates a canonical node or merges canonical
  nodes forces an immediate checkpoint in the same transaction as its report,
  traffic, transitions, node metadata, and redirects.
- Alias refreshes that do not allocate or merge remain replayable journal
  changes and do not independently force a checkpoint.
- Storage failure still cannot replace the committed in-memory state or notify
  SSE subscribers.

## Steps

- [x] Expose canonical-state changes from aggregation without changing the wire
  protocol.
- [x] Force checkpoint and history metadata persistence for those changes.
- [x] Cover random production-style allocation across crash/replay and merge
  checkpointing.
- [x] Update durable checkpoint documentation and complete repository checks.

## Acceptance

- A node first observed less than one second after the prior checkpoint keeps
  the same canonical node and edge IDs after restart.
- Traffic and path history written around that boundary remain attached to the
  restored canonical identities.
- Canonical merge redirects are present in the checkpoint-bearing transaction.
- Ordinary heartbeat and traffic reports remain journal-only inside the
  one-second interval.

## Current state

`ApplyResult` now distinguishes canonical allocations and merges from ordinary
runtime changes. The app forces those reports through the checkpoint-bearing
transaction, while sub-second traffic for already-known identities remains
journal-only. A restart test uses the production random allocator and proves
that node and edge IDs remain stable across checkpoint plus journal replay.

## Next step

No issue-local work remains. Release validation continues in #28.

## Completion summary

PR #47 merged immediate canonical-state checkpoints and production-random
crash/replay coverage into `main`.
