# Project definition

[中文版](project.zh-CN.md)

Tailpath answers one operational question: what is talking to what in a
Tailnet right now, how much traffic is flowing, and which path is Tailscale
actually using?

## In scope

- Discover nodes from trusted observer inventories.
- Collect runtime status from tailscaled and embedded tsnet nodes.
- Reconcile directed observations into logical traffic edges.
- Distinguish direct, DERP, peer relay, and unknown paths.
- Calculate traffic rates from cumulative counter deltas.
- Preserve path events and observation provenance.
- Render a stable live graph and edge details.
- Add Peer Relay, tsnet, and tsbridge observers through one protocol.

## Non-goals

Tailpath is not an ACL visualizer, admin console, packet capture tool, network
manager, active connectivity tester, or full metrics platform. Mobile clients
do not require a Tailpath agent.

## Product boundary

The first stable release targets one self-hosted server, one Tailnet, 250 known
nodes, and 1,000 visible active/recent edges. Missing observations stay unknown;
Tailpath does not infer an unobserved data path.
