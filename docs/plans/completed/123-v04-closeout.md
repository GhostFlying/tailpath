# v0.4 project closeout

Issue: [#123](https://github.com/GhostFlying/tailpath/issues/123)
Parent: [#113](https://github.com/GhostFlying/tailpath/issues/113)

## Goal

Close v0.4 only after an independent read-only review confirms the public
exporter, shared SnapshotSink, native collector migration, tsnet adapter,
multi-runtime example, release gates, and immutable Linux dogfood have no
remaining blocker.

## Scope

- Review the complete v0.4 change set from the final v0.3 closeout commit
  `4bb4c07` through the current `main` head.
- Trace capability negotiation, withdrawal, multi-observer sequencing,
  reconnect/resync, identity replacement, persistence, History, native
  collector compatibility, tsnet adaptation, UI control-traffic handling,
  packaging, scale gates, and dogfood evidence.
- Fix every confirmed closeout blocker before archiving any execution plan.
- Keep issue #58, tags, and GitHub Releases outside this closeout.
- Archive completed plans #113 through #123 only after review and verification
  pass, then close the v0.4 umbrella issue and milestone.

## Acceptance

- Independent review covers the end-to-end protocol, exporter, collector,
  server, storage, web, deployment, and evidence boundaries without changing
  a real Tailnet or repository state.
- No P0/P1/P2 closeout blocker remains unresolved or silently accepted.
- Required focused checks and the full hosted repository gate pass on the final
  closeout PR.
- The v0.4 roadmap, architecture, protocol, data model, security, deployment,
  and evidence documents agree with the shipped behavior.
- No v0.4 execution plan remains under `docs/plans/active/` after merge.
- Issue #58 remains separate; #113 and the v0.4 milestone close only after the
  closeout PR is merged. Tags and releases remain human-only.

## Current state

Complete. The v0.4 implementation and immutable Linux dogfood are merged on
`main`. The final implementation and evidence-preservation changes are present
at `a54a93e`, and post-merge CI
[33204629209](https://github.com/GhostFlying/tailpath/actions/runs/33204629209)
passed on that SHA.
Issue #122 is closed and the sanitized qualification ledger records Direct ->
DERP -> Direct, lifecycle, server/exporter restart, History, no-catch-up,
privacy, and cleanup gates. The independent final review of
`4bb4c07..6c351c0` found no P0 and two P1 blockers: a committed hello whose
response is lost can be omitted from a later explicit withdrawal, and empty
reporter sequence tombstones have no bounded retention. It also found two P2
closeout gaps: public Source cancellation behavior is not documented, and the
post-v0.4 manual scale workflow has not run against the final implementation.
The exporter now retains possibly accepted hello references across ambiguous
transport failures and withdraws them before registration completion or
identity replacement. The server now retains empty reporter sequence
tombstones for two heartbeat intervals and prunes them afterward. Public
Source and RelaySource cancellation obligations are documented.

The first final-scale run on `f1774698` passed every functional and performance
gate but exposed an artifact-retention gap: its second Playwright invocation
replaced the first invocation's `web/test-results` directory. PR #139 assigned
ordinary and Peer Relay gates distinct output directories. The resulting final
workflow on `a54a93e` had two attempts. Attempt 1 accepted all 75,000 reports
without HTTP, server, restart, or memory failure, but a 9.95-second hosted-runner
scheduler stall exceeded the latency thresholds. A single evidence-recorded
rerun on the same SHA passed every gate and retained both browser evidence sets.

## Next step

Complete. Archive plans #113 through #123 in this final closeout PR. Close #123
through the PR, then close #113 and the v0.4 milestone after it merges. Issue
#58, tags, and GitHub Releases remain outside this closeout.

## Verification

- Independent review report against the final v0.4 main head.
- `git diff --check`
- Focused tests selected from review findings.
- Required PR CI, including `make check`, native platform matrix, archives, and
  image gate.
- GitHub issue and milestone state audit after merge.
- Manual scale run
  [33172201058](https://github.com/GhostFlying/tailpath/actions/runs/33172201058)
  on `f1774698`: all gates passed, but the artifact exposed a browser evidence
  retention gap because sequential Playwright runs shared one output directory.
- Final manual scale workflow
  [33204731661](https://github.com/GhostFlying/tailpath/actions/runs/33204731661)
  on `a54a93e`:
  - Attempt 1 accepted 75,000 of 75,000 reports with zero rejected receipts,
    HTTP failures, 500s, request errors, restarts, or OOMs, but failed strict
    latency thresholds after a 9.95-second scheduler stall.
  - Attempt 2 accepted 75,000 of 75,000 reports at 125 reports/s with ingest
    p95 55.498 ms, p99 79.414 ms, scheduler max lag 208.326 ms, and peak RSS
    59,200 KiB. Topology p95 was 9.622 ms, History list p95 173.482 ms, and
    History detail p95 7.145 ms.
  - The seven-day 250-node/1,000-edge database was 707,948,544 bytes.
  - Ordinary browser evidence reported desktop ready in 2,254 ms, visible
    update in 314 ms, and no console errors; mobile ready was 2,351 ms with no
    console errors.
  - Peer Relay browser evidence reported 1,000 sessions through 8 relay
    observers, desktop ready in 2,577 ms and mobile ready in 2,519 ms, with no
    console errors.
  - Artifact `v0.4-multi-runtime-exporter-scale-gate` ID `9699993602` retains
    four browser JSON files, four desktop/mobile screenshots, performance,
    restart, privacy, History-size, cgroup, and container evidence through
    2026-09-27.

Completed on the blocker-fix branch:

- Independent read-only review of `4bb4c07..6c351c0`: no P0, two P1 and two P2
  findings; all code findings are addressed for re-review.
- `go test -race -count=1 ./exporter`
- `go test -race -count=1 ./internal/aggregate`
- `go test -race -count=1 ./exporter ./internal/aggregate ./internal/app`
- `make check` with Go 1.26.5, pnpm 10.15.0, and the existing Playwright
  Chromium installation (28 browser tests passed; 12 scale/mode tests skipped
  by the default gate).
- `git diff --check`

## Completion summary

Independent review and re-review found no remaining P0/P1/P2 code blocker.
Immutable Linux dogfood, post-merge CI, strict performance, restart, History,
privacy, and ordinary/Peer Relay browser evidence all passed on the final main
implementation. The v0.4 execution plans are ready for archival; #58 and
human-only release operations remain separate.
