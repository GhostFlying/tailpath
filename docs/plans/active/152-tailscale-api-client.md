# Add the official Tailscale API client

Issue: [#152](https://github.com/GhostFlying/tailpath/issues/152)
Parent: [#147](https://github.com/GhostFlying/tailpath/issues/147)

## Context

Directory synchronization needs a maintained typed client and OAuth token
renewal without depending on Tailscale's deprecated in-repository client.

## Goals

Pin `tailscale.com/client/tailscale/v2`, construct it with OAuth client
credentials, scope `devices:core:read`, and a 15-second HTTP timeout.

## Non-goals

No synchronization loop, config flags, API keys, or additional scopes.

## Decisions

Wrap the upstream client behind a narrow internal interface so tests use local
OAuth/API servers and later upstream changes do not enter domain types.

## Interfaces

Internal client factory and minimal list-devices interface.

## Steps

Add the module, implement the adapter boundary, and test token acquisition,
renewal construction, timeout, cancellation, and sanitized errors.

## Tests

Local OAuth token server, local Devices endpoint, timeout/cancel, `go mod tidy`,
and `make check`.

## Risks

Default client timeouts or fields can silently expand retention and privilege.

## Current state

Not started.

## Next step

Begin after #157 establishes the accepted boundary.

## Verification

Dependency diff audit, focused Go tests, and `make check`.

## Completion summary

Pending.
