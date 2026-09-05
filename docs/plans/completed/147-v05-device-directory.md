# v0.5 optional device directory

Issue: [#147](https://github.com/GhostFlying/tailpath/issues/147)
Milestone: [v0.5](https://github.com/GhostFlying/tailpath/milestone/7)

## Context

Runtime observers intentionally expose incomplete peer views. v0.5 adds an
optional Tailscale Devices API catalog without weakening Tailpath's passive
data-plane semantics.

## Goals

- Synchronize the credential-visible directory through least-privilege OAuth.
- Reconcile StableNodeID and presentation without inventing traffic or online
  state.
- Provide a separate, responsive Devices workspace with explicit conflicts.
- Preserve last-good data across failures and restarts.

## Non-goals

- Complete Tailnet inventory, API keys, posture attributes, users, ACLs, or
  writes.
- Observer protocol changes, active probes, directory-created Live nodes, or OS
  version.

## Decisions

- ADR 0008 owns source authority and separation.
- `devices:core:read` is the only OAuth scope.
- Directory refresh is a validated full-snapshot replacement.
- Runtime remains authoritative for traffic, path, freshness, observability,
  and online state.

## Interfaces

- Optional server OAuth flags and environment variables.
- `device-directory` capability and `GET /api/v1/devices`.
- Optional `TopologyNode` directory enrichment.
- `/devices` and `/devices/:nodeId` web routes.

## Steps

1. Complete issues #157, #152, #155, #150, #156, #154, #148, #149, #151,
   and #153 in dependency order.
2. Run the existing Live scale gate and the directory fixtures.
3. Dogfood one immutable main image with a real least-privilege OAuth client.
4. Fix every independent review blocker, archive the plans, and close v0.5.

## Tests

Go config, OAuth, synchronization, aggregation, checkpoint, API, fixtures,
Playwright desktop/mobile, scale, restart, stale/recovery, and privacy gates.

## Risks

Control-plane status can be mistaken for runtime online state; NodeKey can
over-merge identities; OAuth responses can leak metadata or credentials; a
failed snapshot can partially replace last-good state.

## Current state

All implementation and blocker-remediation changes are merged on main at
`01d09b8`. Required CI, the manual scale workflow, and the complete real OAuth
qualification pass on its immutable full-SHA image.

## Next step

No implementation or qualification work remains. After the closeout PR merges,
close the remaining v0.5 issues and milestone, revoke the test credentials, and
push the human-owned tag.

## Verification

Each primary issue passes `make check`; the final main stack passes manual
scale, real OAuth dogfood, an independent read-only review, and privacy audit.

## Completion summary

Delivered the optional credential-visible directory, strict runtime/directory
authority boundary, durable reconciliation, responsive Devices workspace,
deployment contract, and privacy-safe real OAuth qualification. Independent
review passed with no remaining P0/P1/P2, and the private lab was deleted.
