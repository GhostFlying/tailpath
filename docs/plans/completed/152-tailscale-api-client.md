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

Complete on main. The official client is pinned at v2.10.1 behind
`internal/devicesapi`, which exposes only approved default device fields,
rejects nil upstream lists, and returns fixed sanitized request errors.

## Next step

No work remains; v0.5 qualification passed and this plan is archived.

## Verification

Dependency diff audit, focused Go tests, and `make check`.

## Completion summary

The adapter uses OAuth client credentials with only `devices:core:read`, a
fixed 15-second HTTP timeout, automatic token renewal, the `-` default Tailnet,
and no API-key construction path. Local OAuth/API fixtures cover approved field
projection, cancellation, renewal, and upstream error-message removal. Focused
race tests and the full repository gate pass.
