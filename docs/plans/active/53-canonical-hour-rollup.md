# Canonical hour-rollup remediation plan

Status: complete
Issue: https://github.com/GhostFlying/tailpath/issues/53
Parent: https://github.com/GhostFlying/tailpath/issues/40
Last updated: 2026-08-24

## Context

The hour tier currently sums minute rows by physical edge before history
queries resolve canonical aliases. If an identity merge splits traffic across
two physical edges in different minutes of one hour, the query-time alias
deduplication keeps only the larger physical-hour total and permanently
undercounts traffic after minute retention expires.

## Decision

- Build each hour from direction-corrected logical minute buckets. Aliases in
  the same minute use a directional maximum; distinct minutes are summed.
- Store the resulting hour under its logical edge ID and persist that generated
  ID as a history edge and identity edge-map entry so later redirect rebuilds
  can continue to resolve it.
- Append migration 4. Existing schema-v3 hour rows cannot reveal whether
  aliases overlapped before aggregation, so migration discards that
  unreconstructable tier and its hour cursor. Maintenance then rebuilds the
  covered range from retained minute rows.
- Preserve raw-to-minute observer selection, two-minute grace, exclusive
  coverage cursors, and cursor-bounded retention unchanged.

## Steps

- [x] Add the schema-v3 semantic repair migration and upgrade coverage.
- [x] Canonicalize minute rows before hour aggregation.
- [x] Persist generated logical hour edges and stable mappings.
- [x] Cover reverse aliases, overlapping and disjoint minutes, minute deletion,
      list/detail queries, and later map rebuilds.
- [x] Update data-model documentation and complete repository checks.

## Acceptance

- Reverse old-alias traffic in an early minute plus canonical traffic in a later
  minute yields the directional sum in the hour tier.
- Multiple physical aliases in the same minute remain deduplicated by
  directional maximum.
- History list and detail retain identical totals after minute rows are deleted.
- A subsequent redirect-map rebuild continues to resolve generated hour IDs.
- Opening a schema-v2 database builds the edge map and applies the hour repair;
  opening schema v3 clears only the unreconstructable hour tier and cursor.

## Current state

Hour maintenance now canonicalizes and direction-corrects each retained minute,
deduplicates aliases within that minute, and sums only across distinct minutes.
The logical hour edge is persisted as a durable history alias. Migration 4
clears unreconstructable schema-v3 hour rows and rebuilds from retained minute
coverage. Store regressions cover overlapping and disjoint reverse aliases,
post-minute-retention list/detail queries, map rebuilds, and schema-v2/v3
upgrades.

## Next step

Run the strict release performance workflow and Linux/macOS dogfood before the
human `v0.2.0-alpha.1` tag decision.
