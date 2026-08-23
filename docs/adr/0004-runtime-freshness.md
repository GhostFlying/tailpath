# ADR 0004: Runtime freshness and receive time

Status: accepted
Date: 2026-08-23
Supersedes: ADR 0002 heartbeat timing

## Decision

Collectors sample locally every two seconds and send traffic reports only when
non-control byte counters change. Idle heartbeats default to one minute.

Server receive time owns current-state ordering, liveness, expiry, and
retention. Collector time is retained as provenance and marked as skewed when
it differs from receive time by more than half a heartbeat interval.

Current path evidence and observer liveness expire after two heartbeat
intervals. Nodes without observer reports or traffic evidence are omitted from
the live topology after four heartbeat intervals. Business traffic is active
for ten seconds; recent edges remain for two heartbeat intervals.

## Consequence

A client with an incorrect clock cannot keep an edge active or delete another
observer's history. Heartbeats never refresh traffic edges. Clients can use the
snapshot's next lifecycle deadline to refresh time-derived state without a new
report.
