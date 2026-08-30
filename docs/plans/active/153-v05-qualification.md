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

Compose packaging, nonroot secret-read CI coverage, the privacy sanitizer and
fixtures, bilingual dogfood runbook, and pending ledger are implemented on the
issue branch. The existing synthetic 250-device and 250-node / 1,000-edge
browser gates pass locally. Real OAuth dogfood remains blocked on the full
stack reaching main and a user-created client with only `devices:core:read`.

## Next step

Run the complete repository check, open the Draft PR, and merge the stack in
order. Then select main's successful full-SHA image and request the OAuth
credential for the more-than-65-minute qualification run.

## Verification

Hosted CI, manual scale workflow, immutable-image dogfood, evidence audit,
independent read-only blocker review, and issue/milestone audit.

## Completion summary

Implementation-side packaging and qualification tooling are complete. Real
OAuth evidence, hosted scale, independent review, plan archival, milestone
closeout, and the human tag remain pending.
