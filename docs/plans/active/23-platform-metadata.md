# Platform metadata and device-node implementation plan

Status: active
Issue: https://github.com/GhostFlying/tailpath/issues/23
Parent: https://github.com/GhostFlying/tailpath/issues/18
Last updated: 2026-08-24

## Context

The live graph currently renders every Tailnet device as the same empty circle.
Runtime telemetry is a top-right badge, while clock skew changes the whole node
border to red. LocalAPI already reports self and peer operating systems, but the
observer contract discards that display metadata. This issue carries optional
platform metadata end to end and gives ordinary devices a stable visual anatomy
without changing identity reconciliation or path-marker semantics.

## Goals

- Add optional `os` to protocol-v1 node identities and live topology nodes.
- Normalize known LocalAPI values to linux, macos, windows, ios, or android and
  preserve unknown non-empty values exactly.
- Keep old reports valid and keep OS out of canonical IDs, aliases, and merge
  matching.
- Render ordinary device nodes as strict 52 by 52 circles with a centered
  platform-derived Lucide device glyph.
- Use a teal top-right runtime-telemetry badge and an amber bottom-right
  TriangleAlert clock-skew badge consistently across graph, Inspector,
  provenance, and legend.
- Preserve DERP, Peer Relay, and Unknown path-specific shapes without invented
  platform glyphs.

## Non-goals

- Detect hardware model, vendor, architecture, ownership, tags, or user.
- Use OS as identity evidence or split/merge nodes when OS changes.
- Change graph layout persistence; #24 owns coordinates and controls.
- Add branded Apple, Windows, Android, or Linux logos.

## Decisions

- `NodeIdentity.os` is an optional free string. Known values are normalized at
  the LocalAPI adapter; unknown values are retained for Inspector display and
  mapped to the generic device glyph.
- Identity reconciliation continues to refresh the stored identity from the
  newest trusted report. A rename or OS change therefore updates display data
  on the existing canonical node without changing its ID.
- `TopologyNode` inherits OS through its existing `NodeIdentity` composition;
  the OpenAPI schema documents the property on both generated shapes.
- Platform presentation is a constant lookup keyed by normalized OS. React
  Inspector and legend use Lucide components directly. Cytoscape receives data
  URIs generated once from Lucide icon nodes, not handwritten duplicate SVGs.
- Ordinary node background layers are center device glyph, optional top-right
  telemetry badge, and optional bottom-right warning. Clock skew no longer
  overloads the primary node border color.
- Relay and path-marker selectors override ordinary node dimensions and clear
  platform background layers.

## Interfaces

- Observer protocol version remains `1`.
- `NodeIdentity.os?: string` and therefore `TopologyNode.os?: string` are
  backward-compatible JSON properties.
- Graph element data adds bounded background image/size/position arrays;
  ordinary node classes retain runtime/peer and skew semantics.
- Inspector adds a Platform detail using the same glyph mapping as the graph.

## Steps

- [x] Add OpenAPI/domain OS fields, regenerate Go/TypeScript types, and prove
  old report compatibility plus identity neutrality.
- [x] Map LocalAPI self and peer OS with known normalization and unknown-value
  preservation tests.
- [x] Add centralized platform presentation and graph element icon data.
- [x] Render strict device anatomy and align Inspector/provenance/legend.
- [x] Add Vitest, Go, and Playwright coverage and inspect desktop/mobile
  screenshots through the documented Playwright fallback.
- [x] Record verification and open the stacked Draft PR.

## Tests

- Generated/domain tests decode old payloads without OS and round-trip new OS.
- Aggregator tests prove an OS update keeps one canonical node and refreshes the
  topology display value.
- Adapter tests cover all five normalized platforms and an unknown value.
- Graph tests cover platform icon mapping, telemetry/skew layers, exact ordinary
  dimensions, and relay/path-marker exclusions.
- Inspector/legend component tests or Playwright assertions verify shared teal
  telemetry and amber warning semantics.
- Desktop and mobile screenshots are checked as nonblank and visually inspected
  because the Browser plugin is unavailable.

## Risks

- Adding OS to inventory hashing would cause an inventory update when display
  metadata changes. That is acceptable but must not affect canonical identity
  or membership; the report still refreshes the same node.
- Cytoscape multi-background syntax varies by property. Tests and screenshots
  must verify all three layers render and remain inside the 52-pixel anatomy.
- Peer Relay may be a real Tailnet node with OS metadata, but when used as an
  explicit path intermediate its path-specific appearance takes precedence.

## Current state

Implementation and local verification are complete. The branch is ready for a
stacked Draft PR based on the #22 collector-resilience branch.

## Next step

Run GitHub Actions on the stacked PR, then retarget it after the dependency PRs
are rebase-merged.

## Verification

- Generated and domain tests prove old JSON remains valid, OS round-trips, and
  platform changes leave canonical identity unchanged.
- Aggregator coverage proves a trusted linux-to-macos refresh keeps one node and
  updates topology display metadata. Adapter tests cover normalized peer OS and
  preservation of unknown values.
- Vitest passes 30 tests, including all five known platform mappings, unknown
  labels, independent telemetry/skew classes, background layers, and Peer Relay
  platform-icon exclusion.
- `go test ./...`, `go vet ./...`, TypeScript/Prettier checks, web tests, and the
  production web build pass. Regenerating API and Lucide assets leaves the tree
  unchanged.
- Playwright Chromium passes four normal desktop/mobile tests and two manual
  250-node/1,000-edge tests. The graph reports every ordinary device at exactly
  52 by 52 model pixels; all image requests load without console errors.
- Browser plugin was unavailable, so repository Playwright Chromium was the
  documented fallback. `view_image` comparison against the accepted node
  anatomy confirms centered platform glyphs, top-right teal telemetry, special
  relay/path-marker shapes, and matching legend semantics. The normal fixture
  intentionally has no clock skew; the scale fixture retains nine warnings and
  the amber TriangleAlert asset/legend.

## Completion summary

Optional platform metadata now flows from LocalAPI through protocol v1 and
canonical topology into a strict device-node anatomy. Platform, runtime
telemetry, clock skew, and path intermediates remain independent visual and data
semantics.
