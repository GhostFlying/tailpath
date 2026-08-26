# Sparse Peer Relay collector reporting

Issue: [#83](https://github.com/GhostFlying/tailpath/issues/83)

## Goal

Extend the existing collector process with capability-detected Peer Relay
sampling and sparse traffic reports while keeping normal tailscaled collection
healthy when relay telemetry is unavailable.

## Decisions

- `relay-telemetry=auto|off` defaults to `auto`; flags override
  `TAILPATH_RELAY_TELEMETRY`, which overrides the built-in default.
- Relay LocalAPI failures are isolated from ordinary collection. They clear the
  relay baseline and emit bounded, sanitized state-transition logs, but do not
  trigger the normal collector retry loop.
- Reporter transport failures retain existing reconnect behavior. The next
  accepted ordinary hello also establishes a fresh relay baseline.
- New sessions, counter resets, removed-and-returned sessions, source ordering
  changes, and every post-gap first sample are baselines. Only positive deltas
  between consecutive healthy snapshots are reported.
- The collector keeps no offline relay queue and never reconstructs catch-up
  traffic.
- `--check` adds only relay capability, enabled state, and session count. Logs
  never include endpoints, scoped client IDs, disco hints, or session IDs.

## Implementation

1. Add optional relay source integration and isolated baseline/delta state to
   the collector.
2. Add CLI/environment configuration and bounded passive diagnostics.
3. Carry `off` through native runner configuration while preserving default
   auto behavior.
4. Cover baselines, positive deltas, resets, removals, reorder, relay failure,
   report failure, reconnect, and sanitized logs.
5. Update protocol, deployment, testing, and parent execution state.

## Verification

- Passed: `go test ./cmd/tailpath ./internal/collector`.
- Passed: Linux and macOS native installer fixtures and shell syntax checks.
- Passed: baseline, delta, reset, removal, transient relay failure, report
  outage, reconnect, config precedence, check privacy, and log privacy tests.
- Pending: hosted Windows PowerShell matrix; no local PowerShell is installed.
- Passed: `CI=1 make check`, including production build and 20 Playwright tests
  with 10 expected project-specific skips. A preceding non-retry run hit the
  known mobile History readiness timeout once; the immediate full E2E rerun
  and CI-mode full gate passed.

## Current state

The collector now samples relay state independently, sends only post-baseline
positive deltas, and carries auto/off through CLI, environment, Compose, and
native runner configuration. Relay failures do not degrade ordinary reports.

## Next step

Review hosted platform results, then merge the stacked PR before #87 begins
server-side scoped reconciliation.
