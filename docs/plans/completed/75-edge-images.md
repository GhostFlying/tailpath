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
- [x] Open PR #76 with the complete workflow and documentation changes; hosted
      PR CI passed while both package-write jobs were explicitly skipped.
- [x] After merge, verify the immutable and mutable multi-architecture manifests,
      OCI revision, and embedded version.
- [x] Import `edge` on the dogfood host, preserve `tailpath-data`, recreate only
      the server, and verify health, identity, history, and collector recovery.

## Current state

PR #76 merged as `e7a0487156df393715644b3dc3bf8862613023d5`, and main CI run
32921998035 published and promoted the first edge artifact. The immutable and
mutable tags resolve to the same multi-architecture digest,
`sha256:4c9f6dfc644dc7569ff85a65d53c369bf825b5c29b2c41f080b0758360107c58`,
with source revision `e7a0487156df393715644b3dc3bf8862613023d5` and embedded version
`edge-e7a0487156df`.

The dogfood host retained the previous `v0.2.0` image as
`rollback-0.2.0-pre-edge`, imported the linux/amd64 edge image, and recreated
only the server. The server is healthy on the unchanged `tailpath-data` volume
and MagicDNS URL. Its tsnet identity was reused, the 24-hour History workspace
retained five prior connections, and all three configured collector runtimes
recovered as online without collector restarts. Playwright verified Live and
History at desktop and mobile viewports, including the new
`3 runtimes reporting` status; both runs had no console errors or warnings.
