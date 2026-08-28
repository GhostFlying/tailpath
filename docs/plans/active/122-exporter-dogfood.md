# Embedded tsnet exporter dogfood

Issue: [#122](https://github.com/GhostFlying/tailpath/issues/122)
Parent: [#113](https://github.com/GhostFlying/tailpath/issues/113)

## Goal

Qualify one immutable `main` OCI artifact in an isolated Tailnet with three
embedded tsnet runtimes, one dedicated reporter identity, and ordinary
application traffic.

## Boundaries

- Never invoke `tailscale ping`, packet capture, preference mutation, or an
  active path-discovery API.
- The workload is an opt-in HTTP transfer from runtime A to a listener owned by
  runtime B. It runs through those existing tsnet stacks and is not part of the
  observer implementation.
- DERP forcing may block non-DNS UDP only inside the disposable exporter
  container network namespace. It never changes host or external-node policy.
- The server and an API-only inspector are additional disposable Tailnet
  identities. Neither is registered as an application observer, and their
  control traffic remains hidden by default.
- Raw topology, identities, endpoints, Tailnet names, and credentials stay in a
  mode-0700 directory outside the repository. Only fail-closed sanitized
  evidence may be committed.

## Artifact contract

- Build `tailpath` and `tailpath-tsnet-multi` in the same Docker build and copy
  both into the runtime image.
- Use only `ghcr.io/ghostflying/tailpath:edge-<full-main-sha>` and record its OCI
  index digest. A PR image, local rebuild, mutable `edge`, or short SHA cannot
  satisfy the final gate.
- Persist server and exporter tsnet state in project-scoped named volumes so
  restart tests reuse identities. The reusable ephemeral key is mounted from a
  mode-0444 file inside a mode-0700 host directory so the nonroot image can read
  the single-file bind mount, then zeroed after all identities enroll.
- Refuse a new qualification when the Compose project already owns containers,
  volumes, or networks, so a prior candidate cannot contaminate state or
  History.
- Persist the validated Compose project in the private runtime file, reject
  later environment mismatches, and revalidate exact mode-0700 evidence
  permissions before every private capture or destructive purge.

## Scenarios

1. Start the server, three runtimes, reporter, and inspector; require exactly
   three application observers online and prove the reporter is not one of
   them.
2. Run the A-to-B workload and require a Direct active edge with positive
   directional traffic and non-empty 15-minute History.
3. Block non-DNS UDP in the exporter namespace, wait passively for the same
   workload edge to become DERP, then restore UDP and require Direct again.
4. Withdraw runtime C, require `2 reporting + 1 stale`, recreate it from the
   same state directory, and require three reporting without traffic catch-up.
5. Stop the Tailpath server for 30 seconds while workload continues; require one
   exporter degraded/recovered transition, stable canonical A-B edge, and
   preserved History without rate or byte growth attributable to offline
   backlog.
6. Restart the exporter process and its dedicated reporter from persisted
   state; require all three runtimes to reappear, stable identities, and no
   catch-up traffic.
7. Scan sanitized topology/History evidence and logs for identity, endpoint,
   auth-key, and payload leaks before retaining the ledger.

## Implementation

- Add the opt-in ordinary workload to `examples/tsnet-multi` with bounded
  errors, explicit connection close, and clean shutdown.
- Package the example binary in the immutable runtime image and validate its
  presence in CI.
- Add an isolated Compose topology, operator helper, fail-closed sanitizer,
  tests, English/Chinese runbooks, and an initially incomplete evidence ledger.
- Execute only after the full stack reaches `main` and its immutable image is
  published successfully.

## Acceptance

- Three application observer nodes and one shared reporter sequence are visible
  through real Tailscale control-plane and data-plane state.
- The dedicated reporter and inspector are not counted as observers.
- Direct -> DERP -> Direct, withdrawal/restart, server outage, exporter restart,
  History persistence, and no-catch-up checks all pass.
- No retained artifact contains credentials, Tailnet names, node identities,
  endpoints, or raw topology.

## Current state

The workload, image packaging, isolated Compose topology, operator helper,
deterministic 30-second outage and continuity/no-catch-up gates, fail-closed
sanitizer, tests, and bilingual runbook are implemented. Local unit, race,
image, Compose, and browser checks pass. The first immutable-main execution
passed enrollment and Direct -> DERP -> Direct, then found a real no-catch-up
failure during the deterministic server outage. A blocking capability request
can recover before the sink consumes snapshots queued during the outage, so it
sends a stale hello and later reports the offline counter growth as traffic.
The History assertion also compares dynamically aligned API buckets by exact
start time and can count pre-outage traffic as new growth.

## Next step

Drain pending source events after capability recovery before constructing the
fresh hello, cover the blocking recovery path under the race detector, and
compare History growth over an explicit post-capture interval rather than by
query-relative bucket identity. Publish a new immutable main image and repeat
the complete qualification before completing the sanitized ledger.
