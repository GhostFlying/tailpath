# Add device copy, conflicts, and Live deep links

Issue: [#151](https://github.com/GhostFlying/tailpath/issues/151)
Parent: [#147](https://github.com/GhostFlying/tailpath/issues/147)

## Context

The workspace needs complete touch, clipboard, conflict, and cross-workspace
behavior after its main structure exists.

## Goals

Copy MagicDNS/IP values on secure and insecure pages, preserve text selection,
show consistent conflicts, and focus a node in Live only when currently visible.

## Non-goals

No StableNodeID copy icon, inferred Live visibility, or hidden-node graph entry.

## Decisions

Use Clipboard API first and a controlled textarea fallback with `aria-live`.
One IP gets one copy control. Conflict detail includes both values and times.

## Interfaces

Copy utility, accessible status announcement, conflict presentation, and Live
focus route state.

## Steps

Implement the shared helpers and UI, then cover pointer, keyboard, touch, HTTP
fallback, conflicts, and conditional Live action.

## Tests

Clipboard success/failure/fallback, selection, IPv4/IPv6, 320px layout,
conflict consistency, visible/hidden Live node, and browser history.

## Risks

Fallback copy can steal focus or break native long-press selection.

## Current state

Not started.

## Next step

Begin after the Devices workspace is reviewable.

## Verification

Playwright interaction tests, accessibility inspection, screenshots, and
`make check`.

## Completion summary

Pending.
