# Synchronize the optional Devices API

Issue: [#154](https://github.com/GhostFlying/tailpath/issues/154)
Parent: [#147](https://github.com/GhostFlying/tailpath/issues/147)

## Context

The server needs optional fail-open configuration, bounded OAuth requests, and
full-snapshot refresh semantics.

## Goals

Validate flags/environment/secret file, sync immediately and every five
minutes, retry failures with bounded jitter, and retain last-good stale state.

## Non-goals

No API key, posture, per-device calls, raw body exposure, or runtime shutdown on
refresh failure.

## Decisions

Flags override environment; tailnet defaults to `-`; client ID and secret file
are all-or-nothing. Retry is 30s, 1m, 2m, 4m, 8m, 15m with 20 percent jitter.

## Interfaces

Server config flags/environment and one cancellable directory synchronizer.

## Steps

Add config parsing, secret validation, minimal upstream conversion, scheduling,
error categorization, invalid-IP counts, and App publication.

## Tests

Config matrix, 401/403/429/5xx/timeout/invalid response, duplicate/empty IDs,
backoff jitter bounds, cancellation, stale/recovery, and token renewal.

## Risks

Raw errors can leak secrets; partial responses can erase last-good data.

## Current state

Implemented on `issue/154-directory-sync`. Server configuration is optional and
strict, and the cancellable synchronizer is connected to the atomic App sink.

## Next step

Open the Draft PR after the full repository gate passes, then expose the
directory and capability through #148.

## Verification

Focused server tests with local OAuth/API servers, race tests, and `make check`.

## Completion summary

Added flags and environment precedence, file-only secret loading, immediate
startup synchronization, five-minute healthy refresh, bounded jittered retry,
sanitized error classes, full-snapshot replacement, and cancellation. Upstream
and validation failures retain the last-good snapshot as stale; disabled
startup durably clears only the current directory layer.
