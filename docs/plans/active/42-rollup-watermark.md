# History rollup watermark remediation plan

Status: active
Issue: https://github.com/GhostFlying/tailpath/issues/42
Parent: https://github.com/GhostFlying/tailpath/issues/40
Last updated: 2026-08-24

## Context

Maintenance currently closes every wall-clock-ended minute immediately and
deletes each traffic tier solely from its retention cutoff. A report whose
server receive timestamp falls into a recently ended bucket can therefore
arrive after that bucket was rolled up, and source data can be deleted even
when the next tier's cursor has not proved coverage.

## Decision

- Raw-to-minute rollup closes only minute buckets ending at least two minutes
  before maintenance time.
- The minute maintenance cursor is an exclusive coverage watermark: every raw
  bucket before it has been represented in the minute tier.
- Hour rollup may close only complete hours before both wall-clock hour end and
  the minute coverage watermark.
- Raw deletion is bounded by both its one-hour retention cutoff and the minute
  cursor. Minute deletion is bounded by both its 48-hour retention cutoff and
  the hour cursor. Hour deletion remains bounded by seven-day retention.
- Rollup, cursor advancement, and covered-source deletion stay in one SQLite
  transaction.

## Steps

- [ ] Apply the two-minute grace to raw-to-minute rollup.
- [ ] Limit hour rollup to complete minute-cursor coverage.
- [ ] Limit source-tier deletion to the next tier's cursor.
- [ ] Cover late arrival, lagging cursor, and retention boundaries.
- [ ] Update durable history documentation and complete repository checks.

## Acceptance

- A raw row added within the two-minute grace is included when its minute is
  eventually rolled up.
- No hour bucket is written until all 60 source minute buckets are behind the
  minute cursor.
- Maintenance cannot delete raw or minute rows beyond the respective next-tier
  coverage watermark.
- Existing directional deduplication and idempotent retry behavior are
  unchanged.

## Current state

Plan opened before implementation.

## Next step

Express the cursor limits directly in rollup and deletion helpers, then add
boundary tests around the approved grace period.
