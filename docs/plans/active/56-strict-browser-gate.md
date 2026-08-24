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
- The hosted workflow remains the authoritative release-gate evidence.

## Interfaces

No runtime, observer, HTTP, storage, or user-facing interface changes.

## Steps

- [ ] Add environment-scoped Playwright worker and retry settings.
- [ ] Move cold-ready sampling to the rendered readiness boundary.
- [ ] Run focused local scale Playwright repeatedly with zero retries.
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

## Next step

Implement the environment-scoped Playwright semantics and rerun the scale gate
without retries.

## Verification

Pending.

## Completion summary

Pending.
