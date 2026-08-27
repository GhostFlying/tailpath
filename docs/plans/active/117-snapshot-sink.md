# Multi-observer SnapshotSink

Issue: [#117](https://github.com/GhostFlying/tailpath/issues/117)
Parent: [#113](https://github.com/GhostFlying/tailpath/issues/113)

## Goal

Run several independent passive observer sources behind one process-level
reporter sequence while isolating source failures, bounding requests, and never
reconstructing traffic across an outage.

## Decisions

- Sample each registered Source concurrently. A single event loop owns
  capability negotiation, batching, sequence, receipts, and reconnect state.
- Healthy sampling defaults to two seconds. Source and transport failures use
  independent jittered exponential retry from two through sixty seconds.
- Batch for 100 milliseconds, up to 64 observers and at most 1 MiB of encoded
  JSON per request. Oversized or invalid source state cannot block siblings.
- Keep only each source's latest in-memory snapshot. Transport failure clears
  delta continuity and requires a complete hello; no durable or catch-up queue
  exists.
- Use one server-owned heartbeat interval for every runtime. Control IDs from
  receipts are excluded before deciding whether business traffic changed.
- Registration withdrawal is explicit and waits for an accepted owner-fenced
  withdrawal while the sink is running. Identity replacement withdraws the old
  observer before hello for the new identity.
- Require protocol 1, `multi-observer`, and `observer-withdrawal` before the
  first report. Incompatibility is terminal; transient transport/status errors
  retry.

## Acceptance

- Three sources produce one batched hello and one monotonically ordered reporter
  sequence; reporter identity is not an observer.
- One source failure, invalid snapshot, or oversized snapshot does not delay
  healthy siblings.
- Counter reset, source recovery, reporter outage, and resync never create
  catch-up traffic.
- Dynamic registration and withdrawal work under the race detector; withdrawal
  and identity replacement preserve ordering.
- Batch observer and encoded-size bounds hold deterministically.

## Current state

SnapshotSink concurrently samples dynamic sources while one event loop owns
capability negotiation, batching, sequence, receipts, reconnect, identity
replacement, and withdrawal. Production timing and bounds are fixed rather
than exposed as public options. Tests cover sparse control-aware deltas, source
timeout/recovery, transport outage without catch-up, resync, dynamic lifecycle,
withdrawal retry, invalid/oversized isolation, rejected-batch splitting, and
observer/JSON limits under repeated race runs.

## Next step

Run the full repository gate, open the stacked PR, and require hosted checks.
Issue #118 will migrate the native ordinary collector onto this same engine.

## Verification

- `go test -race -count=1 ./exporter`
- `go vet ./...`
- `go test ./...`
- `make check`
