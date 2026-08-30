# Checkpoint the directory atomically

Issue: [#156](https://github.com/GhostFlying/tailpath/issues/156)
Parent: [#147](https://github.com/GhostFlying/tailpath/issues/147)

## Context

Last-good directory state and canonical IDs must survive restart without a new
schema migration or partial in-memory publication.

## Goals

Add backward-compatible optional checkpoint state and atomically save one
validated directory candidate plus History metadata before memory/SSE advances.

## Non-goals

No numbered migration, raw upstream response retention, or backup contract.

## Decisions

Use the existing runtime checkpoint JSON. Disabled startup clears the current
directory layer while retaining redirects and History.

## Interfaces

Store transaction for directory checkpoint/metadata and App apply/clear calls.

## Steps

Extend typed clone/restore, add atomic store path, restore legacy checkpoints,
and test rollback and disabled startup.

## Tests

Restart, legacy JSON, storage failure, SSE suppression, deletion, clear-on-
disabled, redirects, and canonical ID stability.

## Risks

A failed commit could publish partial directory state or regenerate node IDs.

## Current state

Not started.

## Next step

Begin after aggregator reconciliation.

## Verification

Store/app race tests, restart fixtures, `git diff --check`, and `make check`.

## Completion summary

Pending.
