# Reconcile device directory snapshots

Issue: [#150](https://github.com/GhostFlying/tailpath/issues/150)
Parent: [#147](https://github.com/GhostFlying/tailpath/issues/147)

## Context

The aggregator must add directory presentation without changing Live runtime
semantics or losing canonical identity on full-snapshot replacement.

## Goals

Apply, replace, and clear directory layers; attach by StableNodeID or safe
NodeKey enrichment; emit effective presentation and explicit conflicts.

## Non-goals

No fetching, storage transaction, HTTP API, or web behavior.

## Decisions

Directory wins name/hostname/OS/display IP while present. Runtime wins online,
observable, path, rate, and freshness. Directory-only nodes are excluded from
Live topology and counts.

## Interfaces

Aggregator ApplyDirectorySnapshot, ClearDirectory, device-directory snapshot,
and optional TopologyNode enrichment.

## Steps

Implement reconciliation, deletion fallback, conflict comparison, canonical
merge rules, redirects, and deterministic output.

## Tests

Rename, deletion, directory-only, runtime peer/reporting runtime, conflicting
StableNodeID/NodeKey, IPv4+IPv6 order, metadata conflicts, and Live invariance.

## Risks

Directory replacement can accidentally create Live nodes or corrupt redirects.

## Current state

Not started.

## Next step

Begin after #155 domain contracts.

## Verification

Focused aggregate/app race tests and existing 250/1,000 Live fixture gate.

## Completion summary

Pending.
