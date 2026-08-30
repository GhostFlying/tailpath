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

Implemented on `issue/150-directory-reconciliation`. The aggregator now keeps
runtime and directory presentation as separate layers, preserves canonical
directory references across identity merges, and excludes directory-only nodes
from Live topology and counts.

## Next step

Open the Draft PR after the full repository gate passes, then build atomic
checkpoint persistence in #156.

## Verification

Focused aggregate/app race tests and existing 250/1,000 Live fixture gate.

## Completion summary

Implemented full-snapshot apply/replace/clear, StableNodeID-first identity,
safe NodeKey placeholder enrichment, explicit identity and metadata conflicts,
effective directory presentation, and a deterministic device-directory view.
Directory addresses never enter runtime IP aliases, and directory presence
never changes runtime online, observable, path, rate, or freshness state.
