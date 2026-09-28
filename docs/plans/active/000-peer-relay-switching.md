# Peer Relay switching and endpoint identity

## Context

Endpoint observers currently report a Peer Relay path as an `ip:port:vni`
string, but Tailpath keeps only the IP for best-effort lookup and removes the
underlay endpoint at the storage boundary. A known relay observation and an
endpoint observation without a StableNodeID therefore become distinct path
evidence. Live chooses one sticky primary, the graph renders one intermediate,
and History cannot reconstruct the endpoint that produced an unresolved path.

The accepted product design keeps every fresh, distinct relay candidate visible
under a relationship-level `Switching` state. Known candidates render as their
real Tailnet node; unresolved candidates render as explicit `Peer Relay`
markers. Relationship traffic remains single-counted.

The operator has explicitly accepted durable storage of public IP addresses
and ports so History can reproduce the evidence that was available at the time.
This supersedes the endpoint-redaction decision in ADR 0006; short disco hints
remain redacted.

## Goals

- Preserve the canonical selected Peer Relay `ip:port` and VNI in reports,
  checkpoints, path events, topology, and History.
- Report fresh public endpoint candidates associated with each observed
  StableNodeID and use only passive status data to resolve a selected relay.
- Resolve by exact relay identity when available and otherwise by a fresh,
  unique public-IP candidate match; ambiguity must remain visible.
- Preserve every fresh, distinct Peer Relay candidate for Live and History and
  expose a relationship-level `switching` path state.
- Render multiple candidates simultaneously without duplicating traffic.
- Keep desktop, mobile, selection, layout-cache, and collision behavior stable.
- Update the protocol, data model, architecture, security contract, and English
  and Chinese operator documentation with the new durable-endpoint behavior.

## Non-goals

- Active ping, packet capture, route probing, preference mutation, or any other
  active discovery.
- Guessing a relay identity when more than one fresh candidate shares a public
  IP.
- Forking sing-box in the first implementation. An embedded Tailpath exporter
  remains a fallback only if endpoint-side passive status cannot supply enough
  evidence.
- Summing traffic across path candidates.
- Rewriting historical events created by older Tailpath versions that never
  stored relay endpoints.

## Decisions

1. `NodeIdentity.publicEndpoints` contains normalized, public `ip:port`
   candidates from `PeerStatus.Addrs` and `CurAddr`. They are evidence and never
   canonical identity aliases.
2. `PathObservation.peerRelayEndpoint` contains the normalized selected relay
   `ip:port`. VNI remains a separate bounded field.
3. The status adapter builds an IP ownership index from fresh peer public
   endpoints. A public IP resolves to a StableNodeID only when exactly one peer
   owns it in that observer snapshot. Tailnet IP matching remains exact. Ports
   are preserved for History but normal Tailscale and relay service ports are
   not required to match.
4. Path evidence uses StableNodeID when known, otherwise endpoint, otherwise
   VNI, otherwise `unknown`. Evidence with the same endpoint is enriched by a
   known StableNodeID before candidate selection.
5. The API exposes `pathState: stable | switching` and `pathCandidates`. The
   existing `path` and `conflicts` fields remain for compatibility and storage
   provenance; candidates are their deterministic normalized projection.
6. A candidate carries path evidence and freshness/provenance sufficient for
   Live and History. Traffic stays on `TopologyEdge`, never on candidates.
7. Relay session endpoints and selected relay endpoints are durable. Short
   disco values continue to be replaced by a presence marker in reports and
   removed from checkpoints/API output.
8. Old databases require no destructive SQL rewrite because report, checkpoint,
   and path-event payloads are typed JSON blobs. New optional fields are
   append-compatible; existing events remain unresolved when their endpoint was
   previously discarded.

## Interfaces

### Observer protocol

```text
NodeIdentity.publicEndpoints[]       normalized public ip:port evidence
PathObservation.peerRelayEndpoint   normalized selected relay ip:port
```

### Topology and History API

```text
TopologyEdge.pathState               stable | switching
TopologyEdge.pathCandidates[]        deterministic current candidates
PathEvent.pathState                  stable | switching
PathEvent.pathCandidates[]           deterministic historical candidates
```

Each candidate contains its `path`, most recent observation time, observer
count, and identity-resolution state. Existing `path`, `conflicts`, and full
observation provenance remain available.

### Accepted design inventory

- Source concept:
  `/data00/home/luchengxuan/.codex/visualizations/2026/09/28/01a0e6af-dbf8-74c3-809d-de6c457a7d46/peer-relay-switching-concept.png`
- Existing Tailpath shell, top bar, filters, graph canvas, inspector, typography,
  square/low-radius controls, and canonical theme tokens remain unchanged.
- Allowed new visible copy: `Switching`, `Path candidates`, `N fresh`,
  `Identity verified`, `Matched by endpoints`, `Identity pending`,
  `Why Switching?`, and the single-counting traffic note.
- Known candidate: canonical relay node anatomy and platform icon.
- Unknown candidate: virtual Peer Relay anatomy with `?`, no invented platform.
- Candidate path distinction: label and anatomy in addition to solid/dashed
  styling; color is never the sole signal.
- Desktop: candidates appear as parallel relay intermediates and as inspector
  rows. Mobile: graph remains usable and the bottom sheet lists candidates.
- Motion: only a restrained switching/dashed-path cue and it must honor
  `prefers-reduced-motion`.

## Steps

1. Extend exporter/domain/OpenAPI contracts and generated types with public
   endpoints, selected relay endpoint, path state, and candidate projections.
2. Update LocalAPI/tsnet normalization to collect public endpoint candidates,
   preserve the selected relay address, and resolve only unique IP ownership.
3. Update path reconciliation so endpoint-equivalent evidence coalesces and
   distinct fresh relay evidence remains deterministic.
4. Persist relay endpoints in raw reports, checkpoints, transitions, anchors,
   and History; retain disco redaction and add backward-compatibility tests.
5. Project candidates into topology and History responses and keep related
   relay node references complete.
6. Implement the accepted multi-candidate graph and Inspector design with
   selection, accessibility, desktop/mobile layout, and no traffic duplication.
7. Extend fixtures and focused unit/integration/e2e tests for known + unknown,
   two known, shared-NAT ambiguity, expiry, restart, and historical replay.
8. Update ADR, protocol, data-model, architecture, security, style guide, and
   bilingual documentation.
9. Run generated-file checks, focused tests, full `make check`, and Playwright
   visual verification at desktop and mobile viewports.

## Tests

- Status adapter: IPv4/IPv6 normalization, private-address exclusion, unique
  public-IP match, shared-IP ambiguity, different normal/relay ports, malformed
  endpoint handling, and full `ip:port:vni` preservation.
- Domain/aggregator: endpoint enrichment, stable-ID precedence, two fresh
  candidates, deterministic ordering, candidate expiry, restart checkpoint,
  relay-session corroboration, and single-counted traffic.
- Store/History: raw endpoint retention, disco redaction, checkpoint replay,
  path anchor/events with candidates, old JSON compatibility, and exact
  selected endpoint restoration.
- Web unit: candidate projection, graph element expansion, logical edge
  selection, filters/counts, labels, and History timeline rendering.
- Playwright: desktop and mobile Switching state, known/unknown candidate
  anatomy, candidate inspector interaction, no horizontal overflow, no console
  errors, and topology retention during candidate convergence.

## Risks

- Public endpoints are sensitive network metadata. The revised security
  contract must state retention, API exposure, and access control explicitly.
- Shared NAT can create false identity matches. Unique ownership within one
  status snapshot is mandatory; conflicting observers keep candidates separate.
- Candidate expansion increases graph geometry pressure. Layout must reserve
  complete intermediate footprints and preserve cached endpoint positions.
- Existing path keys collapse unknown relays. The new endpoint-aware key must
  remain deterministic across restart without treating endpoints as node aliases.
- Tailscale may not expose all candidate endpoints on every platform. Missing
  evidence stays pending; it must not silently downgrade to an IP guess.

## Current state

Implementation complete and verified. Exporter/domain/API contracts now carry
public endpoint candidates, selected relay endpoints, resolution methods, and
deterministic path candidates. The passive status adapter distinguishes exact
Tailscale-IP resolution from unique public-IP endpoint matching while leaving
shared-IP ownership unresolved. SQLite retains endpoint evidence and continues
to redact short disco values.

Live renders all fresh candidates under a relationship-level `Switching`
state, keeps traffic single-counted, and exposes candidate details in the
Inspector. History reconstructs the candidate set stored for each event,
including endpoint, VNI, resolution method, observer count, and observation
time. Desktop and mobile use the accepted concept's known/pending candidate
anatomy.

## Next step

Human review, followed by dogfood on the `r4se-istoreos` to `smallbox` path.
Confirm that a rapid real relay change shows `aliyun-hangzhou-relay` plus any
still-fresh unresolved endpoint, and that the same interval is reproduced in
History. Forking sing-box remains a fallback only if endpoint-side passive
status cannot provide enough evidence in that deployment.

## Verification

- `make check` passed in the repository dev container, including generated-file
  checks, shell/Compose contract checks, gofmt, vet, all Go tests, 72 Vitest
  tests, production build, and the default Chromium E2E suite.
- Default Playwright result: 56 passed and 30 intentionally skipped by project
  configuration, with desktop and Pixel 7 coverage.
- Focused switching E2E passed on desktop and mobile. It checks two visible
  candidates, known endpoint matching, pending identity interaction, traffic
  single-counting copy, zero console errors, and History restoration of both
  endpoints.
- Visual comparison used the accepted concept plus the latest desktop/mobile
  Live and History screenshots.

## Completion summary

Tailpath can now explain rapid Peer Relay switching without requiring the relay
implementation to run an exporter. A known unique public-IP owner is rendered
as its Tailnet node, unresolved endpoints remain explicit candidates, and
durable endpoint evidence makes the same decision inspectable in History.
