# Model directory identity and conflicts

Issue: [#155](https://github.com/GhostFlying/tailpath/issues/155)
Parent: [#147](https://github.com/GhostFlying/tailpath/issues/147)

## Context

Control-plane presentation and synchronization state need typed contracts before
aggregation, storage, and public API work.

## Goals

Define DirectorySnapshot, DirectoryDevice, DirectorySyncState, normalized
platform and MetadataConflict, including deterministic validation and sorting.

## Non-goals

No upstream API types, HTTP synchronization, persistence, or UI.

## Decisions

API nodeId maps to StableNodeID; numeric id is ignored. NodeKey is only a safe
placeholder-enrichment hint. Directory IPs are presentation, not aliases.

## Interfaces

Internal domain types plus optional public TopologyNode enrichment contracts.

## Steps

Add types, normalization, comparison, validation, deep-copy, and JSON tests.

## Tests

Duplicate/empty StableNodeID, platform variants, DNS/address normalization,
NodeKey collision, empty slices, and deterministic ordering.

## Risks

An ambiguous identity hint could merge distinct stable nodes.

## Current state

Not started.

## Next step

Implement after the dependency boundary is available.

## Verification

Focused domain tests, race tests for consumers, and `make check`.

## Completion summary

Pending.
