# Peer Relay Live and History presentation

Issue: [#85](https://github.com/GhostFlying/tailpath/issues/85)

## Goal

Present resolved and unresolved Peer Relay traffic clearly in the existing Live
and History workspaces, using shape, icon, and text in addition to path color.

## Decisions

- Keep the established graph grammar: Peer Relay paths expand into two colored
  segments around an explicit relay node; active/recent remains solid/dashed.
- Ordinary unresolved endpoints remain 52x52 device nodes but use a dashed
  outline and `?`, partial-link, or conflict-alert icon anatomy instead of a
  fabricated platform glyph.
- Display identity status as text plus icon in node Inspector, edge provenance,
  History node labels, and selected path-event provenance.
- Show relay display name, VNI, sanitized session ID, directional rates, and
  A/B/R provenance. Direct endpoints remain visible; Peer Relay underlay
  endpoints remain absent because the API intentionally never exposes them.
- Reuse the current lazy History route and memoized timeline. Derive relay
  presentation from existing payloads without adding another workspace or
  frontend dependency.
- Preserve pan/zoom and avoid full layout on identity changes. New canonical
  nodes use existing neighbor placement and locked incremental layout; relay
  markers remain derived and unpersisted.

## Implementation

1. Add shared identity-status presentation helpers and accessible badges.
2. Update graph node anatomy, relay labels, Inspector details, and legend.
3. Update History edge/timeline provenance with relay/VNI/session/status.
4. Add deterministic routed fixtures for resolved, partial, anonymous, and
   conflict states plus identity replacement without full relayout.
5. Validate desktop/mobile rendering, interactions, console health, and the
   existing History readiness behavior with Playwright.

## Verification

- `pnpm --dir web test`: 51 tests passed, including partial, anonymous, and
  conflict node anatomy.
- `CI=1 ./scripts/e2e.sh`: 26 tests passed across desktop and mobile Chromium;
  10 existing conditional cases skipped. Relay tests cover Live provenance,
  sanitized History provenance, console health, and SSE identity replacement
  without moving the existing neighborhood or viewport.
- Browser/IAB tooling was unavailable. Playwright Chromium was used as the
  documented fallback, and the four desktop/mobile Live/History screenshots
  were inspected with `view_image`.
- Fidelity ledger: Live preserves the established A-to-relay-to-B graph grammar
  and path/activity dimensions; unresolved identity adds icon, outline, and
  text without fabricating a platform. History uses the existing workspace and
  exposes sanitized relay/VNI/session/status provenance. No relay underlay
  endpoint appears in either workspace.
- `PATH=/tmp/tailpath-go/bin:$PATH CI=1 make check`: passed, including generated
  files, Go vet/tests, web type/format/tests/build, and desktop/mobile E2E.
- Hosted PR checks passed: repository check, image build, Linux/macOS/Windows
  native collectors, archive layout, and Conventional Commit title. Edge
  promotion skipped as designed for a non-main branch.

## Current state

Implementation and local browser verification are complete. The graph,
Inspector, History list/detail, path timeline, and legend share one accessible
identity-status vocabulary.

## Next step

Mark the stacked PR ready for human review.
