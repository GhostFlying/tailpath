# Tailpath

Tailpath is a passive, real-time topology and traffic-path viewer for Tailscale
networks. It shows which nodes are communicating, how much traffic is flowing,
and whether each active path is direct, DERP-relayed, peer-relayed, or unknown.

> Tailpath is under active development. The first supported vertical slice is
> tracked in [the roadmap](docs/roadmap.md).

[中文说明](README.zh-CN.md)

## Principles

- Observe runtime state already maintained by Tailscale.
- Never continuously probe peers to discover paths.
- Model traffic relationships, not ACL reachability.
- Preserve observation provenance and represent missing evidence as unknown.
- Keep topology and traffic data inside the self-hosted deployment.

## Planned deployment

The central server runs as a dedicated tsnet node by default. Collectors sample
their local tailscaled status every two seconds and only submit traffic samples
when non-control peer counters change. The server-owned idle heartbeat defaults
to one minute and controls the related freshness windows.

The release artifact is one `tailpath` binary and one OCI image with `server`,
`collector`, and `healthcheck` subcommands. Linux server and collector support
is the v0.2 target. macOS and Windows collectors are buildable, packaged
previews without real-node support claims. Server deployment is supported
through Linux Compose only.

See [deployment](docs/deployment.md) for the native collector installers and
their platform-specific background-service behavior.

Applications embedding several tsnet runtimes can use the alpha public
`exporter` packages. See the runnable
[multi-instance tsnet example](examples/tsnet-multi/README.md).

## Development

Open the repository in its dev container, then run:

```sh
make bootstrap
make check
```

See [development](docs/development.md), [architecture](docs/architecture.md),
and [contributing](CONTRIBUTING.md) before making changes.

## License

Apache-2.0. See [LICENSE](LICENSE).
