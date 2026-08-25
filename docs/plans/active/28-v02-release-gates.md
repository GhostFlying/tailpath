# v0.2 release gates and dogfood implementation plan

Status: active
Issue: https://github.com/GhostFlying/tailpath/issues/28
Parent: https://github.com/GhostFlying/tailpath/issues/18
Last updated: 2026-08-25

## Context

The deterministic scale scenario, incremental checkpoints, stable graph,
bounded history, History workspace, and native collector archives exist on the
v0.2 stack. This issue turns their individual checks into one reproducible
release-candidate gate and records the remaining real-node evidence without
weakening Tailpath's passive runtime boundary.

Tags and GitHub Releases remain human-owned. Performance fixtures may submit
synthetic reports to a loopback-only fixture server. Real dogfood generates
only ordinary application traffic; Tailpath itself never pings, probes,
captures, changes Tailscale preferences, or mutates ACLs or Grants.

## Performance harness contract

- A test-only load command uses the fixed 250-node/1,000-edge scenario. It
  submits one hello per observer, then schedules one report per observer every
  two seconds: 125 reports/s with bilateral provenance over all logical edges.
- The scheduler uses bounded concurrency, never silently drops a scheduled
  request, verifies HTTP 202 plus an accepted receipt, and writes one versioned
  JSON result containing counts, achieved rate, and p50/p95/p99/max latency.
- After ingest, the same command samples topology, history list, and one edge
  detail endpoint and records independent latency distributions. It verifies
  250 canonical nodes and 1,000 logical edges before accepting the run.
- Strict defaults are ten minutes, 125 reports/s, ingest p95 at most 100 ms,
  ingest p99 at most 250 ms, topology p95 at most 250 ms, and history
  list/detail p95 at most 500 ms. Duration and warm-up are overridable only for
  local harness development; the release workflow passes the strict values.
- The server runs in a container limited to 2 CPUs and 512 MiB. A host monitor
  samples the server process RSS and fails above 384 MiB. Server logs are
  retained, and any HTTP 500, rejected receipt, request error, scheduler drop,
  container restart, or OOM is a gate failure.
- The existing seven-day synthetic database test remains the 2 GiB capacity
  gate. Its JSON and resource record are included beside load results.

## Browser gate contract

- The fixed scale Playwright test enforces desktop cold `data-ready` within
  five seconds, exact 250-node/1,000-edge topology, no console errors, cached
  coordinate restoration, and zero automatic layout runs after reload.
- A fixture-only mutation endpoint accepts no caller payload and submits one
  deterministic edge-only report through the normal application transaction.
  Playwright measures trigger response to visible rate update at no more than
  500 ms and verifies layout-run count and existing coordinates do not change.
- The endpoint is registered only by `fixture-server`, remains protected by the
  loopback authorizer, and is absent from production server and OpenAPI routes.
- Screenshots and browser JSON attachments are uploaded with the server/load
  metrics. UI fidelity remains covered by the #26 concept ledger; this issue
  does not redesign the interface.

## Workflow and artifacts

- Rename the manual baseline workflow to the v0.2 release gate while retaining
  `workflow_dispatch` only. Build one commit, run the constrained server and
  load client, run the seven-day database fixture, then run scale Playwright.
- Upload `gate.json`, load/API latency JSON, peak-RSS samples, container inspect,
  server log, database-size output, Playwright JSON attachments, and desktop /
  mobile screenshots for 30 days. No auth key or Tailnet state enters an
  artifact.
- PR CI keeps the existing looser deterministic ingest/restart and browser
  functional smoke; it does not spend ten minutes on strict throughput.
- A local short mode exercises the identical container and parser path before
  the workflow is dispatched.

## Immutable dogfood contract

- A human publishes `v0.2.0-alpha.1`. Record the tag commit, OCI digest,
  archive names, archive SHA-256 values, GoReleaser checksum verification, and
  the performance workflow run. Never rebuild or retag during dogfood.
- Linux container smoke uses the alpha image and an ephemeral reusable auth key
  supplied outside Git. It covers ordinary traffic, Direct to DERP to Direct,
  server restart, collector resync, canonical identity, stale provenance, and
  History list/detail. UDP blocking is opt-in, applies only to disposable test
  namespaces, and always has explicit rollback and cleanup.
- Real Linux dogfood installs the alpha archive and validates systemd,
  `collector --check`, server outage, recovery, and configuration-preserving
  reinstall/uninstall behavior.
- macOS remains preview-only in v0.2. Hosted build, archive layout, and installer
  fixtures are release gates; real arm64 GUI safesocket and LaunchAgent
  qualification is deferred to #58 and is required before any later support
  promotion.
- Windows remains CI-only preview. It is not a release blocker beyond hosted
  build, script parsing, archive layout, and bounded-runner fixture success.

## Review and release decision

- An independent agent/reviewer performs a read-only review of the complete
  v0.2 stack after `alpha.1` dogfood evidence is attached. Review may inspect
  code, artifacts, logs, and screenshots but must not run probes or mutate
  Tailnet state.
- Every blocker receives a focused fix and regression test. Re-run the affected
  strict gate and final `make check`; do not waive thresholds by changing the
  workflow in the fix PR.
- The human release owner alone creates `v0.2.0`. The decision record links the
  exact passing workflow, Linux evidence, macOS preview disposition, independent
  review, resolved blockers, final commit, image digest, and checksums.

## Steps

- [x] Add the deterministic steady-report generator and versioned load result.
- [x] Add ingest/API percentile enforcement and focused unit tests.
- [x] Add constrained Docker orchestration, RSS/health monitoring, and artifacts.
- [x] Add fixture-only edge mutation and SSE-to-visible browser latency gate.
- [x] Enforce the seven-day DB and scale browser gates in the manual workflow.
- [x] Write alpha artifact, Linux smoke, Linux native, and Mac dogfood runbooks.
- [x] Run the local short gate and complete repository checks.
- [x] Open, review, and merge the implementation PRs with passing hosted CI.
- [x] Dispatch the strict workflow from `main` and retain its artifact.
- [x] Human: publish immutable `v0.2.0-alpha.1` artifacts.
- [x] Execute immutable Linux container and native collector dogfood.
- [x] Record macOS as preview and defer real arm64 qualification to #58.
- [ ] Obtain independent read-only review and resolve every blocker.
- [ ] Human: publish `v0.2.0` after the recorded release decision.

## Risks

- A closed-loop client can report low latency while missing the target rate.
  Scheduling is open-loop, bounded, and both scheduled and achieved counts are
  enforced.
- Shared fixture identities can create sequence conflicts. Each simulated node
  retains one stable reporter instance and monotonically increasing sequence.
- A browser can see an SSE event without rendering the fetched topology. The
  gate ends only on a visible edge-rate DOM change, not EventSource receipt.
- Container memory metrics can include page cache or omit child processes. The
  gate records both cgroup usage and the server process RSS, while the approved
  threshold is explicitly process RSS.
- Hosted runner noise may produce an isolated latency miss. Results remain a
  release decision input; thresholds are never auto-relaxed, and reruns retain
  their distinct run IDs.

## Current state

The hosted strict workflow passed for commit `bae04b3`, and the human-published
`v0.2.0-alpha.1` tag resolves to that exact candidate. All six archive checksums
and the immutable OCI image were verified. Linux digest-pinned container and
native collector dogfood passed Direct to DERP to Direct, restart/resync,
History, stale provenance, systemd, outage recovery, and configuration
preservation gates.

An available arm64 macOS 26.3 sandbox provided a logged-in GUI session, but it
had no Tailscale client, denied writes to `/Applications`, and provided no
system-level installation/extension approval path. It cannot satisfy the GUI
safesocket gate and is not accepted as partial real-node evidence. macOS remains
preview-only; #58 owns future qualification.

## Next step

Obtain the independent post-dogfood read-only review. Resolve every blocker,
run final `make check`, and prepare the human release decision with Linux
evidence and the explicit macOS/Windows preview disposition.
