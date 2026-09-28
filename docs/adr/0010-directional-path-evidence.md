# ADR 0010: Preserve endpoint-owned directional paths

Status: accepted
Date: 2026-09-28
Supersedes: ADR 0005 relay path voting semantics

## Context

Each Tailscale endpoint selects its own outbound path. Reconciling both endpoint
reports and relay-session evidence into one sticky edge path can misrepresent a
real asymmetric interval as global switching. Tailscale status also reports
DERP during a short dual-send fallback window without exposing both sends as a
stable product interface.

## Decision

Every logical edge preserves up to two endpoint-owned outbound states. Only an
endpoint observation selects its direction. Relay-session observations may
enrich relay identity and supply fallback traffic evidence but never vote on
the selected path.

A direction carries a primary, optional DERP fallback, and evidence classified
as observed, inferred, or legacy. A process-local monotonic tracker interprets
DERP within twelve seconds of an explicit Peer Relay as a versioned inferred
fallback. Longer DERP is an inferred primary; startup DERP is observed without
guessing a previous path. The inference rule is persisted and exposed.

The existing edge-level primary, conflicts, state, and candidates remain as
compatibility projections. New Live and History views use directional state.
Path events durably store the directional snapshot, and complete history is
available through keyset pagination.

## Consequence

Tailpath can display and reproduce genuine directional asymmetry while keeping
traffic single-counted. Operators can distinguish direct observations from
bounded interpretation. Historical rows created before this schema can expose
only best-effort legacy directions and cannot recover missing transitions.
