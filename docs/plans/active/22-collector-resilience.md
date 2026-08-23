# Collector resilience and diagnostics implementation plan

Status: active
Issue: https://github.com/GhostFlying/tailpath/issues/22
Parent: https://github.com/GhostFlying/tailpath/issues/18
Last updated: 2026-08-23

## Context

The v0.1 collector samples LocalAPI every two seconds, but retries failures on
the same fixed cadence, logs every failure, and can repeatedly resynchronize
after the server has already accepted a complete hello. Its CLI also has no
environment configuration or passive readiness diagnostic. This issue makes
the native collector suitable for unattended Linux and macOS dogfood without
changing observer protocol version 1.

## Goals

- Preserve the healthy two-second sampling cadence.
- Retry LocalAPI and report failures with exponential backoff from two to sixty
  seconds and independent plus-or-minus twenty-percent jitter.
- Send a fresh complete hello from the newest LocalAPI snapshot after a
  transport failure, sequence gap, ownership transfer, or resync request.
- Never queue reports or reconstruct traffic deltas accumulated while offline.
- Support flags over environment over built-in defaults for server URL and
  LocalAPI socket configuration.
- Add a passive one-shot check that reads LocalAPI status but does not report or
  actively probe.
- Keep heartbeat as one server setting constrained to ten seconds through ten
  minutes; all freshness remains derived from it.

## Non-goals

- Add platform metadata to observer protocol or graph nodes; #23 owns that
  schema change. Check diagnostics may report the collector runtime platform.
- Add YAML configuration, persistent offline queues, active Tailscale probes,
  or native service installers; #27 owns packaging.
- Change server authentication or reporter ownership semantics.

## Decisions

- A successful accepted hello is the recovery boundary and resets retry
  backoff. A `resyncRequired` receipt on that complete hello is already
  satisfied by its payload and does not cause another hello loop.
- Any LocalAPI or HTTP failure makes the collector disconnected. The next
  successful LocalAPI read is sent as a complete hello, and that snapshot
  becomes the new traffic baseline whether delivery succeeds or fails.
- An ordinary accepted report with `resyncRequired`, or any rejected report,
  marks the connection for a fresh hello on the next healthy cadence.
- Backoff wait is cancellable. Jitter is sampled uniformly from 0.8 through 1.2
  for each failed attempt; the unjittered exponential value is capped at sixty
  seconds.
- Logging is state-based: one degraded transition, periodic degraded summary,
  one recovery after an accepted hello, and one explicit resync transition.
- `collector --check` emits one JSON object for stable script consumption. It
  creates no reporter and invokes only one `Status` read through LocalAPI.
- CLI flag defaults are derived from environment values, making normal flag
  parsing enforce `flags > environment > built-in default` without hidden
  precedence rules.

## Interfaces

- `TAILPATH_SERVER_URL` configures `--server`; default is
  `http://tailpath:8080`.
- `TAILPATH_SOCKET` configures `--socket`; default is LocalAPI platform
  discovery.
- `tailpath collector --check` prints JSON containing self identity, runtime OS,
  and peer count, then exits.
- Collector options expose retry timing, jitter source, cancellable wait, and
  periodic-summary interval for deterministic unit tests.
- HTTP reporter errors expose status codes while retaining bounded response
  bodies for diagnostics.

## Steps

- [x] Add environment precedence, passive check mode, and CLI tests.
- [x] Implement cancellable jittered exponential retry and transition logging.
- [x] Correct hello/resync behavior and cover reconnect, gaps, resets, skew,
  cancellation, and offline baseline semantics.
- [x] Add typed HTTP status failures and timeout/auth/server tests.
- [x] Enforce and document the ten-second to ten-minute heartbeat range.
- [x] Run Go tests/vet and update project status and verification evidence.

## Tests

- Table tests cover built-in defaults, environment values, and flag overrides.
- Check-mode tests prove one LocalAPI read, valid JSON, and no reporter call.
- A fake clock/wait and deterministic jitter source cover 2s, 4s, 8s through
  the 60s cap and prompt cancellation.
- Collector tests cover LocalAPI errors, transport timeout, 401, 403, 500,
  rejected receipts, resync receipts, server restart, counter reset, reporter
  restart, clock rollback/skew, and no offline traffic reconstruction.
- Log-capture tests assert one degraded transition, bounded summaries, and one
  recovery event rather than per-sample warning spam.

## Risks

- Treating a complete hello's resync receipt like an ordinary sample can create
  an infinite hello loop. Hello acceptance is therefore a distinct transition.
- Retaining a pre-outage baseline would report accumulated bytes as current
  rate. Every disconnected snapshot must replace the baseline.
- Jitter applied after the cap may exceed sixty seconds. The exponential base
  is capped at sixty seconds while the documented jitter applies around it.
- Check mode must not construct an HTTP reporter, since even accidental future
  reporter startup would violate its passive contract.

## Current state

Implementation and local verification are complete. The branch is ready for a
stacked Draft PR based on the #21 incremental-checkpoints branch.

## Next step

Run GitHub Actions on the stacked PR, then retarget it after the dependency PRs
are rebase-merged.

## Verification

- CLI tests cover built-in defaults, environment values, flag overrides, one
  passive diagnostic read, stable JSON output, and heartbeat range rejection.
- Collector tests cover exponential cap and jitter bounds, LocalAPI and report
  failure recovery, accepted-hello resync semantics, ordinary sample resync,
  counter reset, clock rollback, cancellation, offline baseline replacement,
  bounded degraded summaries, and one recovery log.
- HTTP reporter tests cover 401, 403, 500, bounded diagnostic bodies, client
  timeout, direct Tailnet transport, and custom-client preservation.
- Existing aggregator tests retain reporter ownership-transfer and sequence-gap
  coverage. Existing #21 app tests cover server restart and report replay; this
  issue verifies the collector side of an outage as transport failure followed
  by a newest-snapshot hello.
- `go test ./...`, `go vet ./...`, and race-enabled tests for
  `./internal/collector` plus `./cmd/tailpath` pass with Go 1.26.6.

## Completion summary

The collector now has passive diagnostics, explicit environment precedence,
cancellable jittered backoff, fresh-hello recovery without offline delta
reconstruction, state-transition logging, typed HTTP failures, and one bounded
server heartbeat policy.
