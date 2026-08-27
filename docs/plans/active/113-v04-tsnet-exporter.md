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

The v0.4 milestone, umbrella issue, and issues #114 through #123 exist. Issue
#114 adds authenticated capability negotiation. Issue #115 implements explicit
observer withdrawal and advertises that lifecycle only with its persisted
server behavior. The server stores ownership per canonical observer and accepts
multiple observers in one envelope, but the collector engine is still
single-observer and internal. Issue #116 publishes handwritten snapshot,
reporter, and HTTP transport contracts without exposing internal, generated, or
Tailscale types. Issue #117 implements the concurrent SnapshotSink with one
reporter sequence, bounded batching, isolated source/reporter recovery, and
explicit withdrawal. The native collector now uses the shared SnapshotSink for
ordinary LocalAPI state, with Peer Relay as an independently sampled optional
source capability on the same reporter session. Issue #119 now provides a
passive public tsnet Source backed by the same status normalization as the
native collector. Issue #120 adds the runnable three-runtime example with a
dedicated reporting identity and an opt-in withdrawal/restart demonstration.

## Next step

Land the exporter implementation and runnable example, then enforce the
integration, compatibility, UI, and scale gates in #121.

## Verification

- `make check`
- GitHub issue and milestone links resolve.
- The active plan, roadmap, ADR, and repository agent boundary agree.
