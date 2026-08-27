# v0.3 project closeout

Issue: [#82](https://github.com/GhostFlying/tailpath/issues/82)

## Goal

Record the completed Peer Relay implementation, independent review, and real
dogfood qualification, then archive the v0.3 execution plans and close the
milestone. The unresolved arm64 macOS qualification remains a separate,
non-blocking follow-up in issue #58.

## Scope

- Update the v0.3 umbrella and closeout issue with links to merged work and
  retained dogfood evidence.
- Move completed v0.3 active plans to `docs/plans/completed/`.
- Keep tags and GitHub Releases human-only.

## Current state

The v0.3 implementation is merged on `main` at `1329b92`. Main CI passed after
the merge, and the immutable Linux dogfood candidate passed direct, Peer Relay,
restart, recovery, History, provenance, and storage-privacy scenarios. The
independent review found no remaining blocker.

## Next step

Open the documentation PR, run `make check`, and have a human rebase-merge it.
After merge, close issues #82 and #78 and the v0.3 milestone. Keep #58 open for
future tsnet/macOS validation.

## Verification

- `make check`
- Confirm no v0.3 active plan remains after the PR is merged.
- Confirm issue #58 remains outside the v0.3 milestone.
