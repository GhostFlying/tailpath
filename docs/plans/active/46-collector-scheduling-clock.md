# Collector heartbeat clock remediation plan

Status: active
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

- [ ] Separate report telemetry time from local scheduling time.
- [ ] Reset the local deadline for every accepted report kind.
- [ ] Cover LocalAPI timestamp rollback with advancing collector time.
- [ ] Preserve existing sample-duration fallback and reconnect semantics.
- [ ] Update collector documentation and complete repository checks.

## Acceptance

- An idle collector sends within one heartbeat interval even when successive
  snapshot timestamps move backward.
- Report `CollectedAt` values remain the LocalAPI-provided timestamps.
- Traffic still resets the idle deadline and counter/rate behavior is unchanged.
- Existing retry, resync, and cancellation tests continue to pass.

## Current state

Plan opened before collector changes.

## Next step

Replace the snapshot-time deadline with a local scheduling deadline and add the
rollback regression test.
