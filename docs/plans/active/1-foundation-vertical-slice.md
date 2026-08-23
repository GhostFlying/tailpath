# M0 and v0.1 foundation implementation plan

Status: review remediation complete; awaiting CI
Issue: https://github.com/GhostFlying/tailpath/issues/1
Milestones: M0, v0.1
Last updated: 2026-08-23

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

## Confirmed hardening decisions

- Node discovery is permanently best effort unless an operator configures an
  optional control-plane directory source. Runtime observers never promise a
  complete Tailnet inventory, and directory entries never imply traffic.
- An observer inventory is the complete current peer view from that observer's
  LocalAPI or embedded runtime. Its generation is a normalized content hash;
  traffic and path fields do not change it.
- A new inventory replaces that observer's previous peer membership. Removal
  withdraws only that observer's current provenance and never globally deletes
  the canonical node or its history.
- The newest accepted traffic report is the primary current path evidence.
  Path-kind specificity is only a tie-breaker, and fresh conflicts retain
  provenance.
- Server receive time owns liveness, expiry, and retention. Collector time is
  provenance and is marked when it is materially skewed.
- One configurable heartbeat interval owns related freshness thresholds. The
  default is one minute; edge evidence and observer liveness expire after two
  intervals, while nodes without runtime evidence are hidden after four.
- All reporters are trusted, but all application API routes still require a
  Tailnet WhoIs identity. Tailscaled mode rejects broad non-Tailnet listeners
  unless an explicit unsafe reverse-proxy override is supplied.
- The Compose image supports the tsnet server. A system tailscaled collector
  runs natively or in an explicit host-network profile and reaches the server
  by its Tailnet name or address, not by Docker service DNS.
- Default graph edge labels show only one compact aggregate rate. Activity owns
  line continuity: active is solid with current arrows/rate, recent is dashed
  without stale arrows/rate. Path uses color plus relay topology or an Unknown
  marker. Layout edge length is derived from label width with a minimum arrow
  clearance, while a fixed 1.5-5.5px logarithmic scale distinguishes light
  chatter from bulk traffic without per-snapshot rescaling.
- Runtime telemetry is marked on each observed Tailnet/tsnet node, never on the
  reporter process. One trusted reporter may carry several runtime views. The
  legend separates path, activity, and node telemetry.
- Active edges are always visible after path filtering. A browser-persisted
  `Show recent` option defaults on and controls recent edges and their endpoint
  nodes; inventory-only nodes never appear without a visible relationship. The
  empty state describes the currently selected activity window and path.
- Real validation has two stages. Container smoke uses one Linux Docker host,
  one tsnet server, two observable tailscaled sidecars, and one passive
  tailscaled sidecar. A reusable ephemeral auth key enrolls all nodes through a
  temporary Docker secret file that is zeroed after enrollment.
- Container smoke validates real LocalAPI, WhoIs, Direct traffic, sparse
  reporting, provenance, aging, and restart behavior. An explicit test-only
  non-DNS UDP fault in selected disposable namespaces validates real DERP and
  path transitions without changing Tailpath runtime behavior. Cross-host path
  dogfood remains a separate optional operational exercise.

## Container smoke feedback

- [x] Restore one compact aggregate speed to every visible graph edge and use
      a logarithmic traffic-width scale.
- [x] Let the tsnet server read `TS_AUTHKEY=file:/run/secrets/...` so the key is
      not stored in Compose environment values.
- [x] Add an isolated smoke Compose topology with independent Tailscale state
      and LocalAPI sockets for observer A, observer B, and passive node C.
- [x] Add an agent-oriented smoke runner for enrollment, idle capture, ordinary
      HTTP traffic, restart, status, and explicit ephemeral cleanup.
- [x] Replace the three-host-first runbook with container smoke as stage one
      and constrained-network Direct/DERP dogfood as stage two.
- [x] Validate Compose, run the full deterministic suite, and complete the real
      Tailnet smoke with an operator-provided reusable ephemeral key that was
      zeroed locally after enrollment.
- [x] Add an idempotent, reversible non-DNS UDP fault for selected smoke nodes
      and validate Direct -> DERP(hkg) -> Direct using ordinary HTTP traffic.

## Graph legibility follow-up

- [x] Generate and accept complete desktop/mobile concepts and extract the
      implementation design system in `docs/design/v0.1-ui-spec.md`.
- [x] Reserve enough edge length for the compact rate label and arrowheads,
      including relay segments, with a deterministic minimum threshold.
- [x] Reduce the graph edge-label font and remove redundant path text.
- [x] Rename graph node coverage from observer/passive to runtime
      telemetry/peer-only semantics; keep reporter processes out of the graph.
- [x] Encode path and active/recent state as orthogonal visual dimensions and
      split their legend groups.
- [x] Replace the activity segmented filter with a persisted `Show recent`
      option that defaults on and clears hidden recent selections.
- [x] Validate desktop and mobile framing, edge labels, arrows, telemetry
      markers, and the revised legend with Playwright screenshots.
- [x] Keep inventory-only nodes out of the relationship graph and align no
      active/recent/matching empty states with the current UI options.

## Draft hardening steps

- [x] Replace derived string IDs with persisted opaque canonical nodes and an
      alias index; never merge by hostname alone.
- [x] Make inventory replacement, unknown-generation resync, newest-evidence
      selection, freshness expiry, and server receive time explicit in the domain
      and aggregation contracts.
- [x] Persist and restore current reporter, inventory, node, observation, and
      edge state independently of raw history retention; publish SSE only after a
      successful transaction.
- [x] Schedule topology invalidations at lifecycle deadlines and refresh on SSE
      reconnect so active, recent, and observer status age without new reports.
- [x] Parse pinned Tailscale Peer Relay endpoint strings for IPv4 and IPv6 and
      add upstream-shaped contract fixtures.
- [x] Protect every application API with WhoIs, constrain tailscaled listeners,
      and replace the unreachable bridge-network collector Compose service.
- [x] Inject release versions into binaries and images and assert the result.
- [x] Make edge labels compact, hide isolated nodes under path/activity
      filters, and add a non-color-only graph legend. Device icons remain a later
      milestone.
- [x] Run focused tests after each atomic commit, run `make check` locally and
      in GitHub Actions with the pinned toolchain, and inspect desktop and mobile
      screenshots.
- [x] Write a passive v0.1 dogfood runbook for review. Do not execute it until
      the operator approves the topology and commands.

## Independent review remediation

- [x] Keep SQLite and tsnet identity in one named volume and make production
      Compose enrollment file-secret only.
- [x] Prevent equivalent observer-local Direct endpoints from creating path
      transitions and retain a referenced Peer Relay node with an active edge.
- [x] Make baseline observations conform to OpenAPI and define a relay session
      extension that can preserve endpoint and relay-observer provenance.
- [x] Resolve non-blocking lifecycle, naming, validation, and documentation
      drift found during review.
- [x] Run the full deterministic suite and focused delivery checks, then update
      the draft PR and Issue #1 acceptance state.
- [x] Move inventory generation and peer membership from reporter-session state
      to canonical observer runtime state, with hello-based ownership transfer
      and stale-session fencing.
- [x] Migrate persisted draft runtime state in memory, cover collector restart
      and delayed old-session reports, and align the observer protocol/data
      model documentation.
- [x] Make hostname optional in the NodeIdentity OpenAPI contract while keeping
      the domain requirement for at least one stable identity or Tailscale IP.

## History cleanup

- [x] Allow PR and personal branches to use `--force-with-lease` for deliberate
      cleanup while keeping force-push forbidden on `main`.
- [x] Rebuild the verified result as eight subsystem-oriented commits while PR
      #2 remains Draft and has no human review.
- [x] Prove the rewritten head has the intended final tree, rerun deterministic
      checks, and retain a local backup of the prior 49-commit head.
- [x] Replace only the PR branch with an explicit lease and wait for current-head
      GitHub Actions before handing off for human review.

## Steps

- [x] Add project instructions, durable design documents, ADRs, and templates.
- [x] Add OpenAPI, generated-type workflow, and Go workspace.
- [x] Implement sparse reports, aggregation, persistence, and HTTP/SSE APIs.
- [x] Implement the LocalAPI collector and fixture source.
- [x] Implement the React/Cytoscape live topology UI.
- [x] Add dev container, image, Compose, CI, and release configuration.
- [x] Run unit, frontend, container, and smoke verification.

## Verification

- `make check` passed locally with Go 1.26.5, Node 24.19.0, and pnpm 10.15.0.
  It covered generated-file drift, `go vet`, all Go tests, TypeScript,
  Prettier, Vitest, the production web build, and Playwright at 1440x900 and
  Pixel 7 viewports.
- Production static serving, health, and topology API smoke checks passed with
  seven fixture nodes, three observers, and four path relationships.
- Docker Compose v5.5.0 accepted `compose.yaml` with `config --quiet`.
- actionlint v1.7.12 accepted all GitHub Actions workflows.
- GoReleaser v2.17.1 accepted the release config and built snapshot binaries
  for Linux, macOS, and Windows on amd64 and arm64.
- An earlier local Docker attempt stalled while pulling dev-container and
  Dockerfile base layers. GitHub Actions passed the full check with Go 1.26.6
  and built the container image at commit `da54c06`, isolating that failure to
  registry access.
- For the graph-legibility follow-up, the Go 1.26 base dev image completed API
  generation drift, gofmt, vet, all Go tests, and a binary build with the cached
  module set and network disabled. Node 24.19.0/pnpm 10.15.0 completed generated
  TypeScript drift, TypeScript/Prettier checks, 16 Vitest tests, the production
  build, and Playwright at 1440x900 and Pixel 7 (1082x2202 output) viewports.
  Full dev-container feature setup remained blocked by slow GitHub downloads in
  the Docker Compose and nvm feature installers; no product check depended on
  those downloads.
- GitHub Actions also passed the PR-title gate. The current desktop and mobile
  topology screenshots were inspected against the accepted concepts for shell
  composition, graph framing, aggregate speed labels, fixed logarithmic traffic
  widths, path/activity separation, telemetry badges, responsive controls,
  legend placement, and overlap.
- Compose v5.5.0 expanded the nine-service real-Tailnet smoke topology with no
  server port publication and only a secret file path in service environments.
  Shell syntax and actionlint passed. A pre-Docker failure test confirmed that
  the runner zeroes a mode-0600 placeholder key on exit.
- Real Tailnet container smoke passed on 2026-08-23 at commit `2d54f99`. Four
  ephemeral identities enrolled, the server remained tsnet-only with no host
  port, both secret-file copies were zero bytes after enrollment, and the final
  image reported the matching embedded version while running as UID 65532.
- Idle heartbeats produced zero edges. A-to-B and B-to-A transfers reconciled
  into one Direct edge near 2.3 MiB/s with reversed direction and two-observer
  provenance. A-to-C produced one-sided Direct evidence while C remained
  peer-only/runtime-unknown. Stopping B's collector reduced A-to-B provenance
  to one; restarting it restored two observations without a duplicate node.
- Active traffic aged to a zero-rate recent edge and then disappeared at the
  configured lifecycle deadlines without a page reload. Real tailscaled byte
  counters could continue changing for roughly ten seconds after the HTTP
  process exited, so acceptance is measured from the last counter delta rather
  than the workload command's exit time.
- Restarting the tsnet server restored an active edge and both observations
  from SQLite before new reports. The open UI returned to live updates and
  rendered new post-restart Direct traffic. The shutdown path now cancels SSE
  request contexts before graceful HTTP shutdown; the final restart emitted no
  timeout, application warning, collector warning, proxy error, or 403.
- Real-data Playwright checks ran through passive node C's network namespace at
  desktop and Pixel 7 viewports. Active/recent rendering, aggregate speed,
  arrows, path/show-recent filters, mobile search, legend framing, SSE recovery,
  and zero browser warnings/errors passed. Screenshots remained local because
  raw topology evidence can contain Tailnet identifiers and endpoints.
- Traffic generated while the central server is unavailable or while a
  collector is rebuilding its hello baseline is best effort and can be absent
  from deltas. v0.1 does not add a collector-side durable queue; dogfood must
  wait for reporter resync before evaluating post-restart workload traffic.
- The empty-topology regression passed 19 Vitest assertions and four Playwright
  cases across desktop and Pixel 7. With one online inventory node and no edge,
  both activity-option states rendered zero graph nodes and the matching
  `No active traffic` or `No recent traffic` status.
- Controlled DERP smoke passed on 2026-08-23 at commit `b540ad3`. Before fault
  injection, A-to-B was Direct at about 2.10 MB/s with two observations. A/B
  then rejected non-DNS UDP egress for both IPv4 and IPv6 while preserving DNS
  and TCP; ordinary HTTP traffic changed the current path to DERP region `hkg`
  with two observations and sampled application throughput around 161 KB/s.
  Restoring all four firewall states to open and generating new traffic changed
  the same logical edge back to Direct at about 2.25 MB/s. History retained both
  Direct and DERP events, and no active probe was used.
- Independent-review remediation passed generated-file drift, gofmt, `go vet`,
  all Go tests, TypeScript/Prettier checks, 20 Vitest tests, the production web
  build, and four Playwright cases at 1440x900 and Pixel 7 viewports. The
  screenshots and fixture/browser logs were inspected with no overlap, console
  error, or server warning. Production Compose structure, smoke Compose parsing,
  and smoke shell syntax also passed locally.
- A local image rebuild completed the web stage but the Go dependency stage
  could not download `filippo.io/edwards25519@v1.2.0` because access to
  `proxy.golang.org` timed out. GitHub Actions then passed both the full `check`
  job and the authoritative image build at commit `d5c7f26`; the image gate
  also asserted that `/var/lib/tailpath` is its only declared state volume.
- Before updating cleanup status, rewritten commit `c0f0c0e` and policy source
  commit `e6a8a71` had the identical Git tree
  `424c72e8e0dd7e7bb2bee64b636ac7d1fb60e811`. The eight-commit history then
  passed generated-file drift, gofmt, `go vet`, all Go tests,
  TypeScript/Prettier, 20 Vitest tests, the production web build, and all four
  desktop/mobile Playwright cases without application log errors.
- GitHub Actions passed the full `check`, image build/state-volume assertion, and
  Conventional PR title gates on the rewritten eight-commit head `b2bd192`.
- Reporter-session remediation passed focused aggregation vet/tests and the full
  Go suite in the pinned Go 1.26.6 container. Restart replacement, old-session
  fencing, and migration from reporter-owned draft runtime state have dedicated
  regressions. The NodeIdentity contract change passed generated Go/domain
  tests, TypeScript and Prettier checks, all 20 Vitest tests, and the production
  web build. The final deterministic pass also confirmed generated-file drift,
  gofmt, full Go vet/tests, and all four desktop/Pixel 7 Playwright cases.

## Current state

The M0 vertical slice, real Tailnet container smoke, and controlled real-DERP
transition are implemented as atomic commits on
`issue/1-foundation-vertical-slice`. Independent review found deployment,
transition, relay-identity, and protocol-contract gaps; the initial remediation
is complete and all deterministic, browser, Compose, image, and GitHub Actions
gates are green. Codex review then found that reporter-session ownership still
contained observer inventory state and that NodeIdentity incorrectly required a
display hostname in OpenAPI; both are remediated with focused regressions. The
leased cleanup reduced the Draft PR from 49 commits to eight before human review
while preserving the verified product tree.

## Next step

Push the two atomic remediation commits, wait for current-head GitHub Actions,
and resolve the Codex review threads. The cross-host runbook remains available
for a later operational dogfood run, but it does not block v0.1 path correctness.
