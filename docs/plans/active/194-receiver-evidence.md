# Explain receiver traffic with unavailable ingress paths

Issue: https://github.com/GhostFlying/tailpath/issues/194
Branch: `issue/194-receiver-evidence`
Base: `a4b1a9623713e22d2998ca13350c9cd054319e7c`

## Scope and evidence boundary

The user requires official tailscaled only and explicitly accepts unknown paths
when the existing interfaces cannot observe them. Mobile clients need no agent.
Tailscale 1.102.3 internally knows incoming peer and transport, but status exports
per-peer RX counters separately from sender-selected path state. Its inbound
transport metrics are node-wide totals without peer labels. No probes, packet
capture, daemon extensions, preference changes, or metric-based peer attribution
are authorized or needed for this change.

## Implementation

1. Add a shared Web presentation helper for missing directional path evidence.
   Use only the edge's endpoint observations, excluding relay sessions. Distinguish
   sender unknown, receiver-only evidence, and no retained observations.
2. Update `web/src/components/Inspector.tsx` so Live describes who supplies traffic
   counters and explains receiver-only unknown paths. Keep actual paths and traffic
   rates unchanged. Do not claim receiver traffic is active merely because an
   older path observation exists.
3. Update `web/src/history/PathTimeline.tsx` exact-time details using each retained
   segment's own observations. Never use current node online/observable state to
   explain historical coverage. Do not claim traffic at an instant from an entire
   window's byte total or carry old provenance through withdrawals.
4. Document the official-daemon ingress limit and the distinction between traffic
   counters and sender/receiver path evidence in architecture/protocol docs, keeping
   English and Chinese architecture documentation aligned.
5. Add focused helper regressions and desktop/mobile Playwright coverage for a
   receiver-only mobile direction, explicit unknown sender paths, withdrawals,
   and unchanged directional paths/rates. Include these screenshots in the
   existing CI artifact upload for PR review.

## Verification and stop gates

- Focused unit checks must prove that sender paths are never mirrored, relay-only
  provenance cannot identify a receiver, and historical withdrawals remain unknown.
- Use the canonical dev container (`tailpath-devcontainer:playwright`) for pnpm,
  Go, and the final `make check`; generated API files should remain unchanged.
- Browser plugin not available. Use repository Playwright at 1440x900 and 390x844;
  verify interaction, no app errors or horizontal overflow, and save screenshots
  outside the repository for the PR.
- Commit a compiling reviewable Web boundary after focused checks, then open a draft
  PR. Mark ready only after docs, screenshots, and `make check` pass.
- Human review/rebase merge and live deployment are separate follow-up actions.

## Current state

Implementation complete. Live separates TX/RX/relay counter provenance from
directional path state; explicit unknown states are no longer labeled observed.
History identifies retained receiver reports without claiming an exact-time
traffic observation, and withdrawals clear the explanation. English/Chinese
architecture docs and the protocol docs record the official-daemon limit.

Verified in the dev container: Web type/format check, production build, fourteen
focused unit tests, and eight desktop/mobile browser cases pass. Browser cases
exercise receiver-only paths, explicit unknown sender paths, close interactions,
historical withdrawal, console health, and horizontal overflow. Screenshots are
saved outside the repository under the session evidence directory; CI uploads
the same screenshot names with its existing topology artifact.

Next: commit the reviewable boundary and open a draft PR, run the complete
`make check`, inspect final screenshots, and mark ready when all gates pass.
