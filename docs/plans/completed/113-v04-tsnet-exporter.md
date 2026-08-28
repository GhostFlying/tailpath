# v0.4 tsnet exporter execution

Issue: [#113](https://github.com/GhostFlying/tailpath/issues/113)

## Goal

Ship a public Go exporter that treats each embedded `tsnet.Server` as an
independent Tailpath observer while one trusted process-level reporter owns
transport ordering and reconnect. A runnable multi-instance example proves the
same model required by tsbridge without making an external repository a gate.

## Decisions

- Keep the exporter in the existing module under `exporter` and
  `exporter/tsnet`; the public API remains alpha before v1.0.
- Keep Tailpath-owned snapshot types in the core package and Tailscale types in
  the tsnet adapter.
- Use one injectable HTTP reporter per process. The example uses a dedicated,
  persisted reporting tsnet identity.
- Require a v0.4 server through authenticated capability preflight. Existing
  v0.2/v0.3 collectors remain accepted by the v0.4 server.
- Extend protocol v1 with idempotent observer withdrawal. Withdrawal makes the
  observer offline immediately and preserves recent traffic until its ordinary
  freshness deadline; History is unchanged.
- Migrate the native collector to the same ordinary-observer engine. Peer
  Relay session sampling remains an internal extension sharing its sequence and
  recovery state.
- Keep a fixed two-second healthy sampling interval, server-owned heartbeat,
  bounded retry, sparse deltas, and no durable client queue.

## Work items

1. [#114](https://github.com/GhostFlying/tailpath/issues/114): authenticated
   exporter capability negotiation.
2. [#115](https://github.com/GhostFlying/tailpath/issues/115): observer
   withdrawal protocol and persisted lifecycle.
3. [#116](https://github.com/GhostFlying/tailpath/issues/116): public snapshot,
   reporter, and HTTP transport contracts.
4. [#117](https://github.com/GhostFlying/tailpath/issues/117): concurrent
   multi-observer SnapshotSink.
5. [#118](https://github.com/GhostFlying/tailpath/issues/118): native collector
   migration without relay or packaging regression.
6. [#119](https://github.com/GhostFlying/tailpath/issues/119): passive embedded
   tsnet source adapter.
7. [#120](https://github.com/GhostFlying/tailpath/issues/120): runnable
   multi-instance example with a dedicated reporter identity.
8. [#121](https://github.com/GhostFlying/tailpath/issues/121): race, restart,
   browser, compatibility, and scale gates.
9. [#122](https://github.com/GhostFlying/tailpath/issues/122): immutable Linux
   artifact dogfood in an isolated Tailnet.
10. [#123](https://github.com/GhostFlying/tailpath/issues/123): independent
    review, blocker closure, plan archival, and milestone closeout.

## Acceptance

- Three runtimes in one process create three canonical observer nodes and one
  reporter sequence; the reporter identity is not counted as an observer.
- A source failure, withdrawal, identity replacement, process restart,
  reporter outage, or server resync cannot create catch-up traffic or disturb
  healthy sibling runtimes.
- Withdrawal is idempotent, fenced from newer ownership, durable across server
  restart, and changes active traffic only to recent until normal expiry.
- Batches stay below the server's request limit, and a single invalid or large
  observer cannot block the others.
- Existing native collectors, Peer Relay, History, layout, packaging, and the
  250-node/1,000-edge gates do not regress.
- Real Linux tsnet dogfood covers Direct to DERP to Direct, dynamic lifecycle,
  reporter/server restart, History, reporter exclusion, and evidence privacy.
  Cross-layer fixtures and browser gates cover system-telemetry classification
  and default hiding because the dedicated reporter is intentionally not an
  application observer.

## Current state

Complete. Issues #114 through #123 are implemented. The public exporter contracts,
concurrent multi-observer sink, observer withdrawal, native collector migration,
passive tsnet adapter, runnable multi-runtime example, compatibility/scale
gates, and immutable Linux dogfood have all completed. The real dogfood passed
Direct -> DERP -> Direct, reporter exclusion, dynamic lifecycle, server outage,
exporter restart, History persistence, no-catch-up, and evidence privacy. The
sanitized ledger is in `docs/evidence/v0.4-exporter-dogfood.md`. Independent
review found no remaining P0/P1/P2 code blocker after the closeout fixes. The
final manual scale gate passed on `a54a93e` with complete ordinary and Peer
Relay desktop/mobile evidence.

## Next step

Complete. Archive this plan with the v0.4 child and closeout plans. Close issue
#113 and the v0.4 milestone after the archival PR merges. Tags and releases
remain human-only.

## Verification

- `make check`
- GitHub issue and milestone links resolve.
- The active plan, roadmap, ADR, and repository agent boundary agree.
- Manual scale workflow
  [33204731661](https://github.com/GhostFlying/tailpath/actions/runs/33204731661)
  passed on `a54a93e` with the retained artifact
  `v0.4-multi-runtime-exporter-scale-gate` (ID `9699993602`).

## Completion summary

v0.4 delivers the public multi-runtime exporter, native collector reuse,
passive tsnet integration, bounded lifecycle and persistence semantics,
immutable Linux qualification, and final scale evidence. Follow-up issue #58
remains outside this milestone, and no tag or GitHub Release is created by the
closeout.
