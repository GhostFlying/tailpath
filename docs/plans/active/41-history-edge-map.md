# Canonical history edge-map remediation plan

Status: complete
Issue: https://github.com/GhostFlying/tailpath/issues/41
Parent: https://github.com/GhostFlying/tailpath/issues/40
Last updated: 2026-08-24

## Context

Canonical node redirects are currently resolved in Go after history rows are
loaded. Physical edge IDs remain embedded in every traffic/path tier, so a
merge can split one logical edge across aliases, reverse its direction, force
summary queries off the SQL aggregation path, and retain the wrong path anchor.

## Decision

- Append migration 3 with `history_edge_map`, mapping every physical history
  edge to a logical edge, canonical endpoints, and a direction-reversal bit.
- New traffic writes an identity mapping in the report transaction. A newly
  added or changed canonical redirect rebuilds the bounded edge map in its
  metadata transaction; ordinary metadata refresh does not rescan edges.
- Traffic summary SQL first deduplicates physical observer evidence, then takes
  the direction-corrected maximum per logical edge and source bucket, then sums
  time buckets. Redirects therefore retain the indexed summary path.
- Detail queries use the same persisted map for aliases and direction.
- Path-anchor retention chooses one latest pre-cutoff event across every
  physical alias of a retained logical edge.

## Steps

- [x] Add and backfill the append-only edge-map migration.
- [x] Maintain mappings on traffic insert and redirect changes.
- [x] Move summary/detail alias and direction handling onto the map.
- [x] Make path-anchor retention logical-edge aware.
- [x] Cover migration, bilateral deduplication, reversal, and anchor behavior.
- [x] Update data-model documentation and complete repository checks.

## Acceptance

- Pre/post-merge physical edges appear as one logical history edge in nodes,
  list, detail, and path filters.
- Directional bytes remain correct when canonical endpoint ordering reverses.
- Bilateral or alias observations for the same source bucket are not added.
- Redirected history summary queries retain SQL-side aggregation.
- Migration upgrades existing v0.2 dogfood databases without rewriting traffic
  tables.

## Current state

Migration 3 backfills `history_edge_map` without rewriting traffic tables. New
traffic maintains its physical mapping and changed redirects rebuild the
bounded map. Summary SQL and detail queries share direction-corrected alias
deduplication, while path maintenance selects one logical anchor across aliases.

## Next step

Submit the migration and store remediation before the History chart consumes
the corrected sparse time series.
