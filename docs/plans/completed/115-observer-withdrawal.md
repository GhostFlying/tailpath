# Observer withdrawal

Issue: [#115](https://github.com/GhostFlying/tailpath/issues/115)
Parent: [#113](https://github.com/GhostFlying/tailpath/issues/113)

## Goal

Let a trusted reporter explicitly stop owning one or more observer runtimes
without waiting for heartbeat expiry. Withdrawal must make the observer
offline immediately, downgrade its active edge evidence to recent, preserve
History, and remain correct across checkpoint and journal replay.

## Decisions

- Extend observer protocol version 1 with `observer_withdrawal`; the server
  advertises `observer-withdrawal` only with the complete implementation.
- A withdrawal carries observer identity and current inventory generation but
  no peers. It releases ownership only when the sender is the current owner.
- Duplicate, stale-owner, and unknown-observer withdrawals are accepted no-ops
  so an old process cannot withdraw state claimed by a newer hello.
- Persist `withdrawnAt` on observer runtime state and current edge provenance.
  Withdrawn provenance remains available to the checkpoint but is excluded
  from Live reconciliation and rate calculation.
- A later complete hello clears observer withdrawal and establishes a fresh
  baseline. It does not reactivate an edge until new business traffic arrives.
- Withdrawal forces an immediate runtime checkpoint; raw History traffic and
  path events are unchanged.

## Acceptance

- Current-owner withdrawal makes the observer offline and a fresh active edge
  recent in the same topology invalidation.
- Another fresh observer can keep the same logical edge active.
- Repeated, unknown, and stale-owner withdrawals are idempotent no-ops.
- Checkpoint restore and report-journal replay preserve the withdrawn state.
- A new hello can reclaim the observer without reviving old traffic.
- Existing collectors remain accepted without capability preflight.

## Current state

Complete. Protocol v1 accepts peer-free withdrawal envelopes and the capability
endpoint advertises `observer-withdrawal`. The aggregator fences non-owners, records
withdrawal on the observer and current provenance, excludes that evidence from
Live rates and path reconciliation, and forces an atomic checkpoint. Tests
cover immediate active-to-recent behavior, sibling provenance, no-op cases,
checkpoint restore, journal replay, and preserved History.

## Next step

No implementation work remains. Archive this plan as part of the #123 v0.4
closeout.

## Verification

Passed before merge:

- `go test ./internal/domain ./internal/aggregate ./internal/app ./internal/httpapi`
- `make generate`
- `make check`
