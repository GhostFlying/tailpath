# ADR 0003: Dedicated control identity

Status: accepted
Date: 2026-08-22

## Decision

The Tailpath server uses a dedicated Tailnet identity. Its peer edge is system
telemetry, never subtracted from aggregate counters, and hidden from user
activity.

## Consequence

Sharing a tailscaled identity is a degraded opt-in mode because peer counters do
not separate connections or applications.
