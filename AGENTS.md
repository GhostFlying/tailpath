# Tailpath Agent Instructions

## Read Before Editing

Read the assigned GitHub issue, this file, `docs/project.md`,
`docs/architecture.md`, and any relevant ADR or active implementation plan.
Chat history is not a project source of truth.

## Passive Observation Boundary

Tailpath runtime code must not actively probe peers, capture packets, modify
ACLs or Grants, or change Tailscale configuration. In particular, never call
LocalAPI `Ping`, route-probe, capture, or preference mutation methods from a
collector or server workflow.

## Development Environment

Use the dev container as the canonical toolchain. Run `make check` before a PR
is marked ready. Generated API files must be refreshed with `make generate` and
must not be edited by hand.

Tailscale types are confined to `internal/tailscaleadapter`. Domain, storage,
HTTP, and web code use Tailpath-owned types. Released SQL migrations are
append-only.

## Plans And Documentation

Create `docs/plans/active/<issue>-<slug>.md` before work that crosses
subsystems, changes an API or migration, affects security or deployment, spans
multiple PRs, or is expected to take more than one day. Update its current
state, next step, and verification before handing work off.

Update protocol, data-model, deployment, security, or style-guide documents in
the same PR as the behavior they describe. Accepted ADRs are superseded by a
new ADR, not rewritten in place.

## Git And Worktrees

- Keep the primary checkout at `~/WORKSPACE/tailpath` for coordination and
  final verification.
- Use `~/WORKSPACE/tailpath-worktrees/<issue>-<slug>` and branch
  `issue/<issue>-<slug>` for assigned issue work.
- Never overwrite another worktree, branch, plan, or uncommitted change.
- Never force-push `main`, merge, publish a release, or change repository
  settings.
- PR and personal branches may be rewritten for history cleanup with
  `--force-with-lease`, never plain `--force`. Record the expected remote head,
  preserve a local backup ref, prove the replacement tree is equivalent, and
  coordinate with reviewers before rewriting a branch under active review.
- Use the existing user Git identity. Never use an agent/bot identity or add an
  agent/bot/tool co-author.

## Commits And Pull Requests

Use Conventional Commit subjects:

```text
<type>(<scope>): <imperative summary>
```

Allowed types are `feat`, `fix`, `docs`, `refactor`, `test`, `build`, `ci`,
`chore`, and `perf`. Preferred scopes are `server`, `collector`, `web`, `api`,
`store`, `deploy`, and `docs`. Mark breaking changes with `!` and a
`BREAKING CHANGE:` footer.

Keep one primary issue per PR. API plus generated types, migrations plus tests,
and behavior plus required docs belong in the same atomic commit. Do not mix
unrelated formatting or lockfile churn. Draft branches may use incremental
commits, but a ready PR must not contain `WIP` or `fixup!` commits.

Commit a coherent boundary after its focused checks pass and before entering a
different subsystem. Do not leave an entire multi-subsystem issue as one
uncommitted batch. Open a draft PR once the branch has its first compiling,
reviewable implementation commit. Mark it ready only when acceptance criteria,
the active plan, required docs, `make check`, and UI screenshots are complete.

PR titles follow the same Conventional Commit format and summarize the change.
Ready PRs must contain coherent atomic commits because GitHub's Rebase and merge
preserves them on `main`; merge commits and squash merges are disabled. Agents
may open and update PRs for explicitly assigned work, but a human reviews and
rebase-merges them.

## UI Verification

Follow `docs/styleguide.md`. Validate UI changes with Playwright at desktop and
mobile viewports and attach screenshots to the PR. Path meaning must never rely
on color alone.
