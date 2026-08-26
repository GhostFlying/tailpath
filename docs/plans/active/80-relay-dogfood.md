# Real Peer Relay dogfood

Issue: [#80](https://github.com/GhostFlying/tailpath/issues/80)

## Goal

Qualify one immutable v0.3 edge artifact against a real Linux Peer Relay host,
observable endpoint collectors, and passive clients without exposing Tailnet
identity or changing network policy.

## Decisions

- Dogfood starts only after the v0.3 stack reaches `main`, main CI publishes an
  immutable `edge-<full-sha>` image, and the same run uploads native collector
  archives and checksums. A branch build or mutable `edge` pointer is not the
  candidate.
- Upgrade the server first and prove existing protocol-v1 collectors continue
  reporting before replacing the relay collector binary.
- Use anonymous roles in all retained evidence: `S` server, `R` relay, `O1/O2`
  observable endpoints, and `P1/P2` passive endpoints without collectors.
- Generate only ordinary HTTP/iperf application traffic. Never call
  `tailscale ping`, alter ACLs/Grants/Peer Relay policy, block UDP, capture
  packets, or invoke LocalAPI mutation routes.
- Direct-to-Relay-to-Direct uses a human-operated client network-attachment
  change and an already configured Peer Relay. If the Tailnet does not
  naturally select Peer Relay, record the case as not exercised rather than
  forcing it with a policy or firewall change.
- Store raw evidence outside the repository in a mode-0700 temporary directory.
  Commit only the sanitizer, runbook, and a redacted decision ledger containing
  no names, suffixes, addresses, endpoints, stable IDs, session IDs, or keys.
- Retain the old server image and relay collector binary until server, relay,
  collector, and History restart checks pass.

## Implementation

1. Add English and Chinese runbooks with artifact, rollback, scenario, restart,
   and evidence contracts.
2. Add a fail-closed jq sanitizer for topology and collector-check evidence.
3. Validate scripts and synthetic inputs locally.
4. After stacked PRs merge, resolve the exact main CI run and immutable
   artifacts, then execute the real dogfood with a human operating passive
   clients.
5. Record the sanitized result and any unsupported case without weakening the
   acceptance criteria.

## Verification

- Pending: runbook and sanitizer fixture tests.
- Pending: exact main SHA/image/archive record.
- Blocked until execution: relay-host SSH fingerprint confirmation and
  human-operated client traffic/network transitions.

## Current state

The real Tailnet contains an online Linux relay role, but this workstation does
not yet trust its SSH host key. The v0.3 implementation is still a stack of PRs,
so no qualifying main edge image or matching collector archive exists.

## Next step

Implement and locally validate the fail-closed runbook tooling while the v0.3
strict synthetic gate runs.
