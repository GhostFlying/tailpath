# Package and qualify v0.5

Issue: [#153](https://github.com/GhostFlying/tailpath/issues/153)
Parent: [#147](https://github.com/GhostFlying/tailpath/issues/147)

## Context

Optional OAuth configuration must not affect base Compose, leak secrets, or be
declared usable before real token renewal and stale recovery are exercised.

## Goals

Add an optional Compose override, synthetic scale fixtures, privacy-safe
evidence, immutable-image OAuth dogfood, independent review, and closeout.

## Non-goals

No alpha release, package support change, automatic tag, or credentials in
committed files.

## Decisions

Base Compose remains credential-free. Host secret directory is 0700 and the
container-readable file is 0444. Public evidence contains counts, booleans,
latencies, and hashes only.

## Interfaces

Compose override, `.env.example` paths, dogfood runbook/ledger, scale fixtures,
and closeout plan archival.

## Steps

Package config, run 250-device and existing 250/1,000 gates, publish main's
full-SHA image, obtain user OAuth credential, dogfood more than 65 minutes,
review, fix blockers, archive plans, and close milestone.

## Tests

Compose config, nonroot secret read, base start without Devices, first sync,
merge/directory-only, restart, bad-secret stale, recovery, token renewal,
privacy, and final `make check`.

## Risks

Dogfood evidence can leak real names or credential material; shared-device
visibility can be mistaken for completeness.

## Current state

Not started. Real OAuth dogfood requires a user-created client with only
`devices:core:read` after the full implementation is on main.

## Next step

Begin packaging after API and web behavior merge.

## Verification

Hosted CI, manual scale workflow, immutable-image dogfood, evidence audit,
independent read-only blocker review, and issue/milestone audit.

## Completion summary

Pending.
