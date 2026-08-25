# Validated edge image channel

Issue: [#75](https://github.com/GhostFlying/tailpath/issues/75)

## Goal

Publish an OCI dogfood artifact for every fully validated `main` commit while
keeping stable release tags isolated. The local dogfood deployment follows the
mutable `edge` channel only when an operator explicitly upgrades it.

## Contract

- A successful `main` CI run publishes `edge-<full-sha>` for linux/amd64 and
  linux/arm64. The image reports `edge-<short-sha>` and labels the full source
  revision.
- Pull requests, failed main runs, and release tags cannot publish this channel.
- A serialized promotion resolves the newest immutable edge artifact reachable
  from `origin/main` and moves `edge` to that manifest. A late older run cannot
  move the pointer backward.
- `latest` and semantic-version tags remain owned by the release workflow.
- Immutable edge package versions are retained until a separate retention
  policy is reviewed.

## Implementation

- Keep pull-request CI cancellation, but give each main push an independent
  concurrency key so rapid merges do not discard an immutable build.
- Add a package-write job gated on all existing CI jobs. Build and push only the
  immutable tag.
- Add a serialized promotion job. Query GHCR for published edge tags, walk
  `origin/main` newest first, and copy the first matching manifest to `edge`.
- Document the edge channel, version semantics, and manual deployment rollback.

## Verification

- [x] Validate workflow expressions, permissions, event gates, selector behavior,
      and shell syntax locally.
- [x] Run the canonical generated-file, Go, web, build, and browser checks using
      the cached devcontainer-equivalent toolchains.
- [ ] Open a ready PR with the complete workflow and documentation changes.
- [ ] After merge, verify the immutable and mutable multi-architecture manifests,
      OCI revision, and embedded version.
- [ ] Import `edge` on the dogfood host, preserve `tailpath-data`, recreate only
      the server, and verify health, identity, history, and collector recovery.

## Current state

The gated publish and promotion jobs, selector tests, and deployment docs are
implemented and pass local verification. Opening the ready PR and observing its
hosted CI are next. Deployment remains blocked on that PR merging and producing
its first successful edge artifact.
