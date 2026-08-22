# ADR 0001: Passive observation

Status: accepted
Date: 2026-08-22

## Decision

Tailpath reads runtime state already maintained by Tailscale. Runtime workflows
do not actively probe peers, capture packets, or modify network configuration.

## Consequence

Traffic between two unobservable clients remains unknown. This is preferable to
changing the path being observed.
