# Contributing to Tailpath

Tailpath is developed in a dev container so Go, Node, pnpm, code generators,
and browser tooling remain reproducible.

## Workflow

1. Start from an issue with acceptance criteria.
2. Create `issue/<number>-<slug>` in a dedicated worktree.
3. Add an implementation plan when required by `AGENTS.md`.
4. Make focused commits and run `make check`.
5. Open a pull request using the repository template.
6. Wait for human review and squash merge.

## Commit messages

Use Conventional Commits:

```text
feat(collector): emit sparse traffic samples
fix(store): handle reporter counter resets
docs(api): clarify observer heartbeat semantics
```

The allowed types and scopes are documented in `AGENTS.md`. The PR title must
also use this format because it becomes the squash commit and release-note
entry. Do not use bot identities or bot/tool co-author trailers.

## Compatibility

The observer API is versioned under `/api/v1`. Released database migrations are
append-only. Changes to either require fixtures, upgrade tests, and matching
documentation.
