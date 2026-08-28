# Exporter capability negotiation

Issue: [#114](https://github.com/GhostFlying/tailpath/issues/114)
Parent: [#113](https://github.com/GhostFlying/tailpath/issues/113)

## Goal

Let an embedded exporter prove that the authenticated server implements the
v0.4 multi-observer lifecycle before sending any observer report. Older
collectors continue to submit protocol-v1 reports without a preflight.

## Decisions

- Add `GET /api/v1/capabilities` behind the existing WhoIs middleware.
- Return observer protocol versions and feature strings. This issue advertises
  only the already-supported `multi-observer` feature; issue #115 advertises
  `observer-withdrawal` only with its implementation.
- A missing endpoint, malformed success response, unsupported protocol, or
  missing required feature is a permanent incompatible-server error.
- Authentication, transport, timeout, and 5xx errors remain distinguishable so
  the future SnapshotSink can retry them with its normal policy.

## Current state

Complete. The generated contract, authenticated route, Tailpath-owned
capability shape, and HTTPReporter preflight are implemented. The server advertises protocol 1
and only the already-supported `multi-observer` feature. Missing endpoints,
malformed success responses, protocol mismatch, and missing required features
produce a typed permanent incompatibility; HTTP status failures remain typed
and retryable by the caller.

## Next step

No implementation work remains. Archive this plan as part of the #123 v0.4
closeout.

## Verification

Passed before merge:

- `go test ./internal/collector ./internal/httpapi`
- `make generate`
- `make check`
