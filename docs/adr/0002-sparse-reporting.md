# ADR 0002: Sparse traffic reporting

Status: accepted
Date: 2026-08-22

## Decision

Collectors sample locally every two seconds and emit traffic messages only for
non-control peers with counter deltas. Liveness heartbeat defaults to five
minutes and carries no peer activity.

## Consequence

Collectors calculate deltas locally. The server cannot infer real-time rate
from time between sparse reports.
