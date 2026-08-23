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
- Graph element data adds `platformIcon`, `telemetryIcon`, and `clockSkewIcon`
  image values; ordinary node classes retain runtime/peer and skew semantics.
- Inspector adds a Platform detail using the same glyph mapping as the graph.

## Steps

- [ ] Add OpenAPI/domain OS fields, regenerate Go/TypeScript types, and prove
  old report compatibility plus identity neutrality.
- [ ] Map LocalAPI self and peer OS with known normalization and unknown-value
  preservation tests.
- [ ] Add centralized platform presentation and graph element icon data.
- [ ] Render strict device anatomy and align Inspector/provenance/legend.
- [ ] Add Vitest, Go, and Playwright coverage and inspect desktop/mobile
  screenshots through the documented Playwright fallback.
- [ ] Record verification and open the stacked Draft PR.

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

Implementation has not started. The API, aggregator, adapter, and current graph
selectors have been traced end to end.

## Next step

Implement the optional schema/domain field and regenerate both API clients.
