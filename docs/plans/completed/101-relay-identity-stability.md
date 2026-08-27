# Peer Relay identity stability review fixes

Issue: [#101](https://github.com/GhostFlying/tailpath/issues/101)

## Goal

Close the v0.3 independent-review findings that can split one relay client
across identities, discard readable session traffic when optional identity
enrichment fails, or attach directional identity status to the wrong canonical
endpoint.

## Decisions

- A relay client's scoped stable key prefers its trimmed short disco value and
  does not change when full identity evidence appears or its underlay endpoint
  changes. Full identity remains canonical evidence in the payload only.
- When no short disco value exists, the underlay endpoint is the only available
  per-client discriminator in Tailscale v1.102.2. A change therefore creates a
  conservative new baseline rather than guessing continuity.
- The passive relay-session route owns telemetry capability. Failure of the
  optional peer-disco-key lookup degrades identity enrichment independently;
  readable session counters remain available and no catch-up traffic is
  reconstructed.
- Canonical A/B normalization applies to directional bytes and source/target
  identity status in the same branch.
- Protocol-v1 opaque IDs remain trusted reporter input for compatibility.
  Document the residual privacy requirement that exporters generate opaque
  IDs rather than embedding endpoint or hostname material. A future protocol
  revision may enforce or server-rekey their format.
- Extreme VNI reuse inside the freshness window remains a documented upstream
  lifecycle risk. Counter reset and client-key changes establish a baseline,
  but v1.102.2 exposes no explicit session-removal event.

## Implementation

1. Stabilize adapter client keys and ordering across enrichment and endpoint
   drift, with missing-short fallback tests.
2. Represent identity-enrichment availability separately from relay session
   capability and preserve sessions across 403, 500, malformed, and transport
   failures.
3. Carry bounded degraded/recovered diagnostics through the collector without
   resetting the relay counter baseline.
4. Normalize identity status with canonical edge direction.
5. Add collector and App/checkpoint/restart coverage for anonymous-to-resolved
   continuity, redirects, traffic direction, and no catch-up behavior.
6. Update protocol, data model, security, testing, and v0.3 execution state.

## Verification

- Passed: focused adapter, collector, aggregator, application, store, and
  collector-check tests.
- Passed: race tests for adapter, collector, aggregator, application, and
  store packages.
- Partial: `make check` passed generated-file, formatting, vet, Go/web unit,
  and production-build gates. Its four-worker browser run exposed the existing
  Vite-proxy/SSE starvation in the mobile navigation test; the same test passed
  with one worker. Hosted CI remains the authoritative full gate.
- Passed: hosted `make check`, native collector, and image jobs for the first
  implementation commit.
- In progress: strict relay gate and independent collision-finding closure on
  the final implementation head.

## Current state

The stable-key, enrichment-degradation, and canonical-direction fixes are
implemented with focused and cross-layer restart coverage. Protocol and
security documentation record the trusted opaque-ID and extreme VNI reuse
residual risks. Independent follow-up found one collision ambiguity; the
adapter and aggregator now reject that evidence without producing a self-edge.

## Next step

Complete. The collision fix and cross-layer restart coverage were merged and
included in the v0.3 independent review.
