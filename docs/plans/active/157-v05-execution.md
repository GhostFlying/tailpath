# Open v0.5 execution

Issue: [#157](https://github.com/GhostFlying/tailpath/issues/157)
Parent: [#147](https://github.com/GhostFlying/tailpath/issues/147)

## Context

The milestone needs durable authority, security, execution, and delivery rules
before implementation branches change public interfaces.

## Goals

Open the milestone and issues, accept ADR 0008, update bilingual durable docs,
and create one active plan per primary issue.

## Non-goals

No dependency, runtime, API, UI, Compose, or database implementation.

## Decisions

Directory data is optional control-plane enrichment, not runtime observation.
Rebase merge, atomic commits, human tags, and synthetic public evidence remain
mandatory.

## Interfaces

Documentation and GitHub project state only.

## Steps

1. Create milestone, umbrella, and primary issues.
2. Add ADR 0008 and update project, architecture, data model, security,
   deployment, style guide, and roadmap.
3. Add active plans and verify links and formatting.

## Tests

`git diff --check`, Markdown/link inspection, and repository `make check`.

## Risks

Later PRs can diverge if conflict, stale, or identity authority is underspecified.

## Current state

Milestone #7 and issues #147-#157 are open. ADR 0008, the bilingual durable
documentation updates, and one active plan per primary issue are complete.

## Next step

Open the Draft PR and make it ready after hosted CI passes.

## Verification

GitHub milestone/issue audit, `git diff --check`, and `make check`.

## Completion summary

The v0.5 authority, security, identity, API, UI, delivery, and dogfood boundaries
are documented. Local `make check` passed in the canonical devcontainer with 56
web unit tests and 30 default Playwright tests; 12 manual scale cases were
skipped by the normal gate.
