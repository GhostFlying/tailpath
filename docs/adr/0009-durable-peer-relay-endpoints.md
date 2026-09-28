# ADR 0009: Persist Peer Relay endpoint evidence

Status: accepted
Date: 2026-09-28
Supersedes: ADR 0006 endpoint volatility and redaction requirements

## Context

Standard Tailscale status exposes a selected Peer Relay as an underlay
`ip:port:vni` value without a reliable relay StableNodeID. Tailpath previously
used that endpoint only during current report processing and removed it from
reports, checkpoints, path events, History, APIs, and logs. That protected
underlay metadata but made an unresolved historical path impossible to
reconstruct and prevented a later UI from explaining rapid relay switching.

Some embedded relay implementations cannot run a Tailpath exporter. Endpoint
observers can still passively see the selected relay endpoint and may see fresh
public endpoint candidates for Tailnet peers. Retaining that evidence permits a
deterministic, bounded identity correlation without probing or modifying the
network.

## Decision

Tailpath durably stores normalized public `ip:port` candidates reported by
runtime observers and the selected Peer Relay `ip:port` plus VNI. Relay session
client endpoints are retained in the report journal when supplied; endpoint
evidence required by current node/path state is retained in the runtime
checkpoint. The same evidence may appear in authenticated topology and History
API responses and operator-visible diagnostics. Tailpath still never emits
outbound product telemetry.

Public endpoints are evidence, not global identity aliases. A selected relay IP
may resolve to a StableNodeID only when one fresh observer snapshot associates
that public IP with exactly one peer. A port match may strengthen evidence but
is not required because the ordinary Tailscale and Peer Relay service ports are
different. Shared-IP or cross-observer disagreement remains unresolved.

Short disco hints remain sensitive scoped material. Raw report storage replaces
them with a constant presence marker, and checkpoints, History, API responses,
and logs omit their value.

All collection remains passive. Collectors read status already maintained by
Tailscale and never ping, route-probe, capture packets, or change preferences.

## Consequence

Live and History can reproduce the relay endpoint and the distinct candidates
that were fresh during a switching interval, including unresolved candidates.
An embedded relay can often be identified from two endpoint observers without
embedding a Tailpath exporter. Older events whose endpoints were already
discarded remain unresolved.

The SQLite database now contains public network metadata. Operators must treat
the database, backups, authenticated API responses, and diagnostic exports as
sensitive Tailnet operational data and apply the same access and retention
controls as traffic history.
