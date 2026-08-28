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

The v0.4 implementation and immutable Linux dogfood are merged on `main`.
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
Source and RelaySource cancellation obligations are documented. The final
manual scale run on `f1774698` passed every functional and performance gate,
but its second Playwright invocation replaced the first invocation's
`web/test-results` directory. The uploaded artifact therefore retained the
Peer Relay browser evidence but not the ordinary 250-node/1,000-edge browser
evidence.

## Next step

Preserve each Playwright invocation in a distinct output directory and rerun
the manual scale workflow on the resulting `main` SHA. Record the complete run
and artifact in a separate final closeout PR. Only that PR archives plans #113
through #123 and closes #123; #113 and the milestone close after it merges.

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
