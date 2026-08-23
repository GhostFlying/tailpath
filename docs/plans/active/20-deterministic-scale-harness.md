# Deterministic scale harness implementation plan

Status: active
Issue: https://github.com/GhostFlying/tailpath/issues/20
Parent: https://github.com/GhostFlying/tailpath/issues/18
Last updated: 2026-08-23

## Context

The v0.1 fixture is intentionally small and animated. It cannot establish the
250-node/1,000-edge product boundary or provide identical input to ingest,
restart, history, and browser tests. This issue adds a deterministic scale
scenario before any performance optimization.

## Goals

- Generate exactly 250 canonical nodes and 1,000 logical edges from a fixed
  seed, with two endpoint observations per edge.
- Cover Direct, DERP, Peer Relay, and Unknown paths, active and recent edges,
  asymmetric traffic, and materially clock-skewed reporters.
- Reuse one report model for app ingest/restart, API, and browser fixtures.
- Add a reliable PR-scale functional smoke and an opt-in detailed workflow that
  records machine-readable baseline evidence and screenshots.

## Non-goals

- Optimize current JSON cloning, runtime-state checkpoints, SQLite retention,
  or Cytoscape layout.
- Assert the final v0.2 latency and memory thresholds in ordinary PR CI.
- Generate traffic on a real Tailnet or invoke any active Tailscale probe.

## Decisions

- The graph is a degree-eight ring: each node connects to the next four nodes,
  producing `250 * 8 / 2 = 1,000` unique logical edges without random retries.
- Every node has its own deterministic reporter instance. Hello and traffic
  batches therefore exercise ownership and sequence handling realistically.
- The fixed seed assigns rate, path details, endpoint strings, and skew
  membership. Tests assert stable digests and aggregate counts rather than
  depending on incidental map order.
- The existing animated fixture remains the default. `fixture-server --scale`
  loads the scale scenario once for browser and manual baseline runs.
- Browser validation uses repository Playwright because the Browser plugin is
  unavailable in this session.

## Interfaces

- `fixtures.NewScaleScenario(config)` returns immutable scenario metadata and
  deterministic hello/traffic report batches.
- `fixture-server --scale` selects the bounded scale fixture; it is rejected on
  the production `server` command.
- The manual benchmark workflow accepts dispatch inputs and uploads JSON plus
  Playwright screenshots. It records baselines in this issue and does not gate
  PRs on final release thresholds.

## Steps

- [ ] Implement and unit-test deterministic scenario generation.
- [ ] Add app ingest/restart functional scale coverage.
- [ ] Expose the scale browser fixture and assert rendered data counts.
- [ ] Add a lightweight PR smoke target and manual benchmark workflow.
- [ ] Document the baseline, commands, and Playwright fallback.

## Tests

- Unit tests verify node/edge/provenance/path/state/skew counts and a stable
  scenario digest.
- App tests ingest all reports, compare topology before and after SQLite
  restart, and sample history from every path class.
- Playwright waits for graph readiness at desktop scale and captures evidence.
- CI runs the functional scale smoke; workflow dispatch records elapsed time,
  RSS, database size, API timings, and screenshots without enforcing RC gates.

## Risks

- Current per-report full-state cloning may make 500 sequential reports too
  slow. The harness must measure that honestly without hiding it through batch
  shortcuts.
- A full 1,000-edge COSE layout may exceed ordinary PR timeouts. The browser
  smoke should use an explicit generous functional timeout and report elapsed
  time; #24 owns layout optimization.
- Determinism can be lost through canonical IDs or map iteration. Tests use a
  deterministic node-ID allocator and sorted summaries.

## Current state

The issue and this active plan are open. Existing small fixture and test entry
points have been inspected; implementation has not started.

## Next step

Implement the domain-level scenario generator and its exact-count/digest tests.

## Verification

Pending.

## Completion summary

Pending.
