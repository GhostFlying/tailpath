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

The full stack is on main at `f0eb253`, its hosted repository and manual scale
workflows passed, and real OAuth dogfood is running against the immutable
full-SHA image. First sync, directory/runtime reconciliation, directory-only
Live isolation, restart, stale last-good, recovery, and real desktop/mobile UI
checks passed. The more-than-65-minute token-renewal window is in progress.

Independent review found two qualification blockers before closeout: an
upstream HTTP 200 response with a nil device list can be accepted as an empty
snapshot, and the evidence content hash includes volatile runtime enrichment.
It also found three P2 correctness/privacy gaps: directory-first runtime
evidence can expose a zero timestamp, MagicDNS copy removes the trailing dot,
and the Devices response is cacheable. All five will be fixed and requalified
on a new immutable main image.

## Next step

Reject nil upstream device lists as `invalid-response`; hash only
directory-authoritative fields in the dogfood sanitizer; use a valid identity
evidence timestamp without changing Live freshness; copy the full MagicDNS
value; mark the Devices response `no-store`; and add regression tests. Then run
`make check` and open the blocker-remediation Draft PR. After it merges, repeat
the immutable-image OAuth qualification before final ledger archival.

## Verification

Hosted CI, manual scale workflow, immutable-image dogfood, evidence audit,
independent read-only blocker review, and issue/milestone audit.

## Completion summary

Implementation-side packaging and qualification tooling are complete. Real
OAuth evidence, hosted scale, independent review, plan archival, milestone
closeout, and the human tag remain pending.
