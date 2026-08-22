# M0 and v0.1 foundation implementation plan

Status: in progress
Issue: bootstrap
Milestones: M0, v0.1
Last updated: 2026-08-22

## Context

Tailpath starts from an empty repository. This plan establishes the documented,
agent-operable project foundation and a fixture-driven vertical slice before
real Tailnet dogfooding.

## Goals

- Establish repository, agent, documentation, development-container, CI, and
  release conventions.
- Define the versioned sparse observer protocol.
- Implement the server, SQLite persistence, topology aggregation, SSE, and web
  graph.
- Implement a passive LocalAPI collector with control-peer filtering.
- Provide deterministic tests and Compose-based local operation.

## Non-goals

- Peer Relay server-side session telemetry.
- The public tsnet/tsbridge exporter package.
- Tailscale Devices API enrichment.
- Production release or automatic merge.

## Steps

- [ ] Add project instructions, durable design documents, ADRs, and templates.
- [ ] Add OpenAPI, generated-type workflow, and Go workspace.
- [ ] Implement sparse reports, aggregation, persistence, and HTTP/SSE APIs.
- [ ] Implement the LocalAPI collector and fixture source.
- [ ] Implement the React/Cytoscape live topology UI.
- [ ] Add dev container, image, Compose, CI, and release configuration.
- [ ] Run unit, frontend, container, and smoke verification.

## Verification

Pending implementation.

## Current state

Repository is empty apart from Git metadata.

## Next step

Add M0 project constraints and development scaffolding.
