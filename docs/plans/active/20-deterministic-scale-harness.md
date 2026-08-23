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

- [x] Implement and unit-test deterministic scenario generation.
- [x] Add app ingest/restart functional scale coverage.
- [x] Expose the scale browser fixture and assert rendered data counts.
- [x] Add a lightweight PR smoke target and manual benchmark workflow.
- [x] Document the baseline, commands, and Playwright fallback.

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

Implementation and local verification are complete. The branch is ready for a
stacked Draft PR based on the #19 governance branch.

## Next step

Run GitHub Actions on the stacked PR, review the recorded baseline boundary,
and mark the PR Ready after #19 is merged and the base is retargeted to `main`.

## Verification

- Fixed-seed contract tests pass with 250 nodes, 1,000 logical edges, 2,000
  directed observations, four equally represented paths, 666 active edges, 334
  recent edges, and nine clock-skewed observers.
- Full app-to-SQLite ingest and restart passed in the Go 1.26.6 container. The
  unoptimized baseline was about 23 seconds and 5.37 MB for one scale load.
- `go test ./...` and `go vet ./...` passed in the Go 1.26.6 container.
- `pnpm --dir web check`, unit tests, and production build passed with Node
  24.19.0 and pnpm 10.15.0.
- Default fixture Playwright passed four desktop/mobile tests. Scale Playwright
  passed desktop and Pixel 7 with 1,000 logical edges and nonblank screenshots;
  `data-ready` took about 17-19 seconds.
- Browser plugin was unavailable, so repository Playwright Chromium was the
  explicit fallback. The known SSE fetch-abort resource warning is captured in
  the baseline and remains owned by #21.

## Completion summary

The deterministic scale scenario, persistent smoke, browser fixture, and manual
baseline workflow are implemented without optimizing production behavior.
