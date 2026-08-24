# Workspace connectivity remediation plan

Status: complete
Issue: https://github.com/GhostFlying/tailpath/issues/45
Parent: https://github.com/GhostFlying/tailpath/issues/40
Last updated: 2026-08-24

## Context

`WorkspaceTopbar` defaults to a green `live` state. History therefore claims a
live server even though it opens no SSE stream and can simultaneously show HTTP
request failures. The label conflates two different availability signals.

## Decision

- The topbar requires an explicit typed connection presentation; it has no
  optimistic default.
- Live presents topology SSE state as Connecting, Live, Reconnecting, or
  Unavailable. Topology request failure is tracked separately from SSE state so
  a successful retry can reveal the current stream state immediately.
- History presents the HTTP state required by its current desktop/mobile view
  as Connecting, Reachable, or Unavailable.
- A required index or detail request failure makes History unavailable. A retry
  clears the error while connecting and restores reachable only after success.
- Accessible labels name the workspace signal rather than a generic green dot.

## Steps

- [x] Make topbar connectivity explicit and typed.
- [x] Map Live SSE state without changing reconnect behavior.
- [x] Derive History HTTP state from required index/detail requests.
- [x] Cover initial load, failure, retry recovery, desktop, and mobile.
- [x] Update History documentation and run rendered verification.

## Acceptance

- History never renders an unconditional green `live` state.
- A failed History request visibly and accessibly reports unavailable.
- Successful retry changes the same indicator back to reachable.
- Live reconnecting/error behavior remains driven only by SSE/topology state.

## Current state

The shared topbar now requires a typed presentation and has no default. Live
maps its SSE state plus topology request health, while History derives the state
from the index and detail requests required by the current responsive view.
Playwright covers History HTTP failure/recovery, Live topology retry recovery,
accessible labels, and responsive status.

## Next step

Submit the focused connectivity remediation before the collector scheduling
clock fix.
