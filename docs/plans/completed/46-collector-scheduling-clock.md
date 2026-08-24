# Collector heartbeat clock remediation plan

Status: complete
Issue: https://github.com/GhostFlying/tailpath/issues/46
Parent: https://github.com/GhostFlying/tailpath/issues/40
Last updated: 2026-08-24

## Context

Heartbeat scheduling currently stores and subtracts LocalAPI snapshot
`CollectedAt`. If that reported wall clock moves backward, an idle observer can
remain below the heartbeat interval indefinitely and age out on the server.

## Decision

- `CollectedAt` remains telemetry data time for reports, clock-skew marking,
  last-active evidence, and traffic sample-duration calculation.
- Heartbeat scheduling uses only the collector's local `Now` clock captured
  after each snapshot read and after successful sends.
- Production `time.Now` comparisons retain Go's monotonic component. If an
  injected/non-monotonic scheduler clock moves backward, the collector sends a
  conservative heartbeat rather than postponing forever.
- Hello, inventory, traffic, and heartbeat acceptance all reset the scheduling
  deadline; failed/rejected sends do not.

## Steps

- [x] Separate report telemetry time from local scheduling time.
- [x] Reset the local deadline for every accepted report kind.
- [x] Cover LocalAPI timestamp rollback with advancing collector time.
- [x] Preserve existing sample-duration fallback and reconnect semantics.
- [x] Update collector documentation and complete repository checks.

## Acceptance

- An idle collector sends within one heartbeat interval even when successive
  snapshot timestamps move backward.
- Report `CollectedAt` values remain the LocalAPI-provided timestamps.
- Traffic still resets the idle deadline and counter/rate behavior is unchanged.
- Existing retry, resync, and cancellation tests continue to pass.

## Current state

Heartbeat scheduling now records collector-local time after each snapshot and
accepted send. `CollectedAt` remains untouched for report telemetry and traffic
duration. The rollback regression drives LocalAPI timestamps backward while the
collector clock advances and proves the heartbeat still fires at 30 seconds.

## Next step

No issue-local work remains. Release validation continues in #28.

## Completion summary

PR #52 merged collector-local heartbeat scheduling and rollback regression
coverage into `main`.
