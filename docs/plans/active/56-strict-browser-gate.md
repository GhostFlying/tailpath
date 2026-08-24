# Deterministic strict browser gate remediation plan

Status: active
Issue: https://github.com/GhostFlying/tailpath/issues/56
Parent: https://github.com/GhostFlying/tailpath/issues/28
Last updated: 2026-08-25

## Context

The first hosted v0.2 release-gate workflow completed successfully only after
the desktop scale test retried twice. Cold-ready measured 5,006 ms and 5,472 ms
while desktop and mobile cold layouts ran concurrently, then 3,324 ms when the
final desktop retry ran without that contention. The generic CI configuration
allows two retries and full parallelism, which is useful for functional tests
but can mask a strict performance miss.

The test also computes `readyElapsedMs` after an additional topology request
and data assertions, rather than when the graph first reports `data-ready`.

## Goals

- Measure one cold desktop browser without concurrent mobile layout work.
- Make any scale performance threshold miss fail without retry masking.
- Capture cold-ready at the exact rendered readiness boundary.
- Preserve desktop/mobile rendering, cached reload, SSE visibility, stable
  coordinates, screenshots, and machine-readable metrics.
- Make repeated local gate runs wait for fixture and Vite port release rather
  than connecting to an aging process from the preceding run.

## Non-goals

- Do not raise or otherwise weaken the five-second cold-ready threshold.
- Do not change graph layout behavior or production React code unless an
  isolated strict run still misses the accepted threshold.
- Do not change ordinary PR Playwright parallelism or retry behavior.

## Decisions

- When `TAILPATH_SCALE_E2E=1`, Playwright uses one worker and zero retries.
- Other CI Playwright runs retain the current fully parallel, two-retry policy.
- `readyElapsedMs` is sampled immediately after the `data-ready=true`
  assertion and before independent topology-contract checks.
- Fixture and Vite run in independent process groups. E2E cleanup terminates
  and waits for each complete group; Vite uses its requested port strictly so
  a cleanup regression fails instead of silently using another port.
- The hosted workflow remains the authoritative release-gate evidence.

## Interfaces

No runtime, observer, HTTP, storage, or user-facing interface changes.

## Steps

- [x] Add environment-scoped Playwright worker and retry settings.
- [x] Move cold-ready sampling to the rendered readiness boundary.
- [x] Make repeated E2E process cleanup and port ownership deterministic.
- [x] Run focused local scale Playwright repeatedly with zero retries.
- [ ] Run repository checks and hosted PR CI.
- [ ] Re-run the hosted strict v0.2 release gate from `main`.
- [ ] Record the passing artifact in #28 and archive this plan.

## Tests

- Focused desktop/mobile scale Playwright with `TAILPATH_SCALE_E2E=1`.
- Existing normal desktop/mobile Playwright to prove unchanged PR behavior.
- Full generated, Go, Web, build, and browser repository checks.
- Hosted strict workflow with one worker, zero retries, and retained JSON and
  screenshots.

## Risks

- Serial projects increase browser-step duration but remain far below the
  45-minute workflow timeout.
- Capturing readiness earlier can hide later topology-contract failures; those
  assertions remain mandatory and fail independently.
- An isolated desktop miss would indicate real graph performance work is still
  required and must not be papered over by test changes.

## Current state

Hosted run 32748704571 passed service, restart, history-size, and browser jobs.
Its browser annotation records two desktop threshold misses before a final
isolated retry passed. The retained final metrics report cold ready at 3,324 ms,
cached ready at 1,957 ms, and visible SSE update at 377 ms.

The remediated local gate completed three consecutive one-worker, zero-retry
runs. Desktop cold ready measured 2,490/2,559/2,712 ms and visible SSE update
measured 381/343/340 ms. Desktop and mobile screenshots were nonblank with the
expected 250 nodes, 1,000 logical edges, controls, and legends. Two consecutive
normal E2E runs each passed 13 applicable tests and released all fixture/Vite
ports before the next run.

## Next step

Run full repository checks, obtain hosted PR CI, and rerun the strict workflow
from the merged commit without retries.

## Verification

- `pnpm --dir web check`: passed.
- `pnpm --dir web test`: 46 passed.
- `pnpm --dir web build`: passed.
- Three focused scale runs: two projects passed per run with one worker and no
  retries; all desktop cold-ready and SSE metrics passed their thresholds.
- Two consecutive normal E2E runs: 13 passed and seven configured skips each.
- Desktop/mobile screenshots inspected at original size.

## Completion summary

Pending.
