# Peer Relay integration and scale gates

Issue: [#89](https://github.com/GhostFlying/tailpath/issues/89)

## Goal

Turn the v0.3 Peer Relay data path into deterministic integration, persistence,
browser, and scale gates without weakening the existing v0.2 performance
baseline.

## Decisions

- Keep the existing 250-node/1,000-edge mixed-path fixture and 125 reports/s
  gate unchanged so results remain comparable with v0.2.
- Add a separate 250-canonical-node/1,000-concurrent-session relay fixture.
  Eight relay runtimes report third-party sessions through real
  `relay_session_update` envelopes; the result remains 1,000 client-to-client
  logical edges rather than relay-to-client business edges.
- Use synthetic documentation-safe identities, endpoints, and session IDs.
  Persistence tests scan the database, WAL, checkpoints, logs, and exported
  evidence for endpoint/disco canaries.
- Keep semantic cases small and explicit: relay-only, one-side and two-side
  resolution, conflict, reset/session removal, restart/resync, and
  Direct-to-Relay transition. The scale fixture measures the resolved hot path
  rather than conflating ambiguity with throughput.
- PR CI runs bounded functional ingest/restart/privacy and browser smoke. The
  detailed resource and latency gate remains `workflow_dispatch` and uploads
  JSON, resource measurements, and screenshots.
- Preserve the passive boundary. Fixtures submit already-observed runtime
  state and never probe a peer, alter Tailscale configuration, or capture
  packets.

## Implementation

1. Add the deterministic relay scale generator and contract tests.
2. Exercise App ingest, checkpoint restart, History provenance, traffic
   de-duplication, and persistence redaction for 1,000 sessions.
3. Add a relay-scale fixture-server mode and browser assertions for explicit
   relay anatomy, stable layout, and sanitized API payloads.
4. Add bounded PR smoke and a v0.3 manual strict workflow with retained
   artifacts.
5. Run the mixed-path and relay-specific gates, update testing documentation,
   and record the measured baseline.

## Verification

- Relay generator contract, aggregation, SQLite/checkpoint restart, History,
  de-duplication, and database/WAL/log/export privacy tests pass for 250 nodes,
  eight relay observers, and 1,000 sessions.
- `CI=1 TAILPATH_RELAY_SCALE_E2E=1 ./scripts/e2e.sh`: desktop and mobile passed;
  desktop cold/cached ready measured 2,610/2,186 ms and mobile 2,376/1,797 ms,
  with no console errors.
- `CI=1 TAILPATH_SCALE_E2E=1 ./scripts/e2e.sh`: the unchanged mixed-path
  desktop/mobile baseline passed (two project-specific cases skipped).
- Sequence-gap resync and durable Direct-to-Peer-Relay transition tests pass.
- Pending: v0.3 hosted strict workflow dry run.
- Pending: `make check` and hosted PR checks.

## Current state

The separate relay generator, fixture-server mode, persistence smoke, and
browser gate are implemented. The original mixed-path fixture remains
unchanged for comparable performance results.

## Next step

Run repository-wide checks, then open the stacked PR and execute the hosted
manual workflow.
