# Contributing to Tailpath

Tailpath is developed in a dev container so Go, Node, pnpm, code generators,
and browser tooling remain reproducible.

## Workflow

1. Start from an issue with acceptance criteria.
2. Create `issue/<number>-<slug>` in a dedicated worktree.
3. Add an implementation plan when required by `AGENTS.md`.
4. Make focused commits and run `make check`.
5. Open a pull request using the repository template.
6. Wait for human review and rebase merge.

Do not wait for the whole issue before opening a PR. Open it as a draft after
the first compiling, reviewable implementation commit so API and architecture
direction can be reviewed early. Convert it to ready only after every acceptance
criterion is met, the active plan and durable docs are current, `make check`
passes, UI screenshots are attached when relevant, and no `WIP` or `fixup!`
commit remains.

PR and personal branches may be reorganized before review with
`git push --force-with-lease`; never use plain `--force` and never rewrite
`main`. Preserve a local backup and verify that the cleaned history produces the
same final tree before replacing a published PR branch. Coordinate any rewrite
after human review has started so reviewers do not lose their place.

## Commit messages

Use Conventional Commits:

```text
feat(collector): emit sparse traffic samples
fix(store): handle reporter counter resets
docs(api): clarify observer heartbeat semantics
```

The allowed types and scopes are documented in `AGENTS.md`. The PR title must
also use this format and summarize the linearly rebased change. Do not use bot
identities or bot/tool co-author trailers.

Commit each coherent boundary once its focused tests pass and before moving to
a different subsystem. Keep generated API types with the contract, migrations
with their tests, behavior with required docs, and dependency changes with the
code that needs them.

## Compatibility

The observer API is versioned under `/api/v1`. Released database migrations are
append-only. Changes to either require fixtures, upgrade tests, and matching
documentation.
