# ADR 0007: Separate embedded observers from reporter transport

Status: accepted
Date: 2026-08-28

## Context

A Go process may contain several independent `tsnet.Server` instances. Each
instance has its own Tailnet identity, peer view, counters, path state, and
lifecycle. Requiring each instance to maintain an independent Tailpath
transport duplicates ordering and reconnect state, while treating the process
as one observer loses the topology Tailpath exists to show.

## Decision

Tailpath models every embedded tsnet runtime as an independent observer. A
process-level SnapshotSink may report several observers through one trusted
HTTP reporter. Reporter identity authenticates transport with WhoIs but is not
observer identity and is not counted as an observable runtime.

The core exporter uses Tailpath-owned snapshots. A separate tsnet adapter owns
Tailscale types and passive LocalAPI conversion. One reporter instance owns a
single sequence and reconnect state; inventory generation, counter baseline,
heartbeat, and source health remain per observer.

The v0.4 exporter requires capability preflight and explicit observer
withdrawal. Withdrawal releases only the current reporter's ownership, makes
the observer offline immediately, and lets recent traffic age normally without
deleting canonical identity or History. A stale reporter cannot withdraw an
observer claimed by a newer reporter.

## Consequence

tsbridge-style applications can add and remove runtimes without creating a
process-shaped graph node or coupling sibling failures. Applications need one
persisted Tailnet transport identity for the shared reporter, and v0.4
exporters must connect to a v0.4 server. The public package remains alpha until
the v1.0 API compatibility review.
