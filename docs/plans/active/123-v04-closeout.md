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
privacy, and cleanup gates. The independent final review has not started.

## Next step

Run an independent read-only review of `4bb4c07..origin/main`. If it finds a
blocker, fix and re-review it before closeout. Otherwise record the review,
archive plans #113 through #123, open the closeout PR, and require all hosted
checks before closing #113 and the v0.4 milestone.

## Verification

- Independent review report against the final v0.4 main head.
- `git diff --check`
- Focused tests selected from review findings.
- Required PR CI, including `make check`, native platform matrix, archives, and
  image gate.
- GitHub issue and milestone state audit after merge.
