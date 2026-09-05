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
container-readable file is 0444. Public evidence may contain only the candidate
SHA, full-SHA image tag, image-reported version, OCI digest, workflow links,
scale artifact ID and expiry, generic platform versions, durations, counts,
booleans, latencies, fixed error categories, and sanitizer-produced
identity-set, content, and canonical-mapping hashes.

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

The five independent-review findings were fixed in #173 and re-reviewed with
no remaining P0/P1/P2. Main commit `01d09b8` passed required CI and the manual
scale workflow and published its immutable full-SHA image. First sync,
directory/runtime reconciliation, directory-only Live isolation, checkpoint
restart, invalid-secret stale last-good, recovery, privacy scanning, real
desktop/mobile UI checks, and the more-than-65-minute token-renewal gate passed
on that artifact. Final ledger review found no P0/P1/P2, and the isolated lab
and private evidence were deleted. The final canonical `make check` passed.

## Next step

Open the closeout PR. After it merges, close #147, #153, #168, and milestone
v0.5; the user revokes the OAuth client and reusable enrollment key before
pushing the human-owned tag.

## Verification

Hosted CI, manual scale workflow, immutable-image dogfood, evidence audit,
independent read-only blocker review, and issue/milestone audit.

## Completion summary

Implementation, blocker remediation, hosted CI, hosted scale, immutable-image
OAuth qualification, independent ledger review, plan archival, and private lab
cleanup are complete, and the final repository gate passed. The closeout PR,
issue/milestone closure, external credential revocation, and human tag remain
workflow actions.
