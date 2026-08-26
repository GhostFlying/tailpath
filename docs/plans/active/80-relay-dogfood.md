# Peer Relay dogfood

Issue: [#80](https://github.com/GhostFlying/tailpath/issues/80)

## Goal

Qualify one immutable v0.3 edge artifact against real Tailscale control-plane,
WireGuard, Peer Relay, LocalAPI, and application-traffic behavior in an
isolated container lab. A physical Linux relay remains useful deployment
evidence, but is not a prerequisite for validating the v0.3 telemetry path.

## Decisions

- Dogfood starts only after the v0.3 stack reaches `main`, main CI publishes an
  immutable `edge-<full-sha>` image, and the same run uploads native collector
  archives and checksums. A branch build or mutable `edge` pointer is not the
  candidate.
- Enroll disposable Tailscale containers with a reusable ephemeral key, then
  zero the staged key before generating workload traffic. The key, node names,
  Tailnet suffix, addresses, and raw reports never enter retained evidence.
- Configure one disposable `tailscaled` as a real Peer Relay and require the
  existing control-plane grant to expose it to the disposable clients.
- Make Direct -> Peer Relay -> Direct deterministic only inside disposable
  network namespaces: the relay phase permits DNS and the relay UDP port while
  rejecting other UDP. Tailpath runtime code never probes or mutates Tailscale.
- Use collector start/stop to cover two-observable, one-observable, and
  relay-only traffic without changing the underlying session.
- Use anonymous roles in all retained evidence: `S` server, `R` relay, `O1/O2`
  observable endpoints, and `P1/P2` passive endpoints without collectors.
- Generate only ordinary HTTP/iperf application traffic. Tailpath runtime and
  host operations never call `tailscale ping`, alter ACLs/Grants, capture
  packets, or invoke LocalAPI mutation routes. The disposable harness may apply
  only the namespace-local UDP rules defined below.
- The disposable harness may mutate only its own namespace firewall and Relay
  preferences. It must restore or destroy those namespaces on every exit and
  must not mutate host firewall rules, ACLs, Grants, or unrelated Tailnet nodes.
- Store raw evidence outside the repository in a mode-0700 temporary directory.
  Commit only the sanitizer, runbook, and a redacted decision ledger containing
  no names, suffixes, addresses, endpoints, stable IDs, session IDs, or keys.
- Retain the old server image and relay collector binary until server, relay,
  collector, and History restart checks pass.

## Implementation

1. Add English and Chinese runbooks with artifact, cleanup, scenario, restart,
   and evidence contracts.
2. Add a fail-closed jq sanitizer for topology and collector-check evidence.
3. Extend the disposable Compose smoke topology with a relay node, relay
   collector, deterministic relay-only UDP mode, and cleanup traps.
4. Validate Direct -> Peer Relay -> Direct, all three observability scenarios,
   relay collector/tailscaled/server restarts, Live/History, and endpoint
   persistence privacy against the immutable main candidate.
5. Record the sanitized result without weakening the acceptance criteria.

## Verification

- Runbook and sanitizer fixture tests passed on the pre-rebase tree and retain
  identical tree content after rebase.
- Candidate main SHA, image digest, archives, and checksums are resolved.
- A separate disposable Tailnet passed Direct -> Peer Relay -> Direct, all
  three provenance scenarios, collector/relay/server restarts, History, and
  database endpoint scans using real Tailscale v1.102.2 traffic.
- The run exposed missing top-level relay identity when a newer endpoint
  observation agreed with more detailed relay provenance. Issue #107 and PR
  #108 contain the isolated fix and regression coverage; the corrected branch
  runtime passed the three-party reproduction.
- Pending: merge #108, select its immutable main artifact, and repeat the final
  artifact-bound matrix before accepting the ledger.

## Current state

The repeatable harness and preliminary real-control-plane run are complete.
The reusable credential files were zeroed immediately after enrollment, and no
production or long-lived node was modified. The final immutable-main run waits
only for the independently scoped reconciliation fix in #108.

## Next step

Merge #108, rebase this PR, rerun the exact immutable main image in the existing
isolated Tailnet, accept the redacted ledger, and destroy all ephemeral state.
