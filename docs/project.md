# Project definition

[中文版](project.zh-CN.md)

Tailpath answers one operational question: what is talking to what in a
Tailnet right now, how much traffic is flowing, and which path is Tailscale
actually using?

## In scope

- Discover known nodes best effort from trusted observer peer views. A complete
  Tailnet directory is never implied unless an optional control-plane directory
  source is configured, and directory entries never imply traffic. Even then,
  Tailpath promises only the devices visible to that directory credential, not
  a complete Tailnet inventory.
- Collect runtime status from tailscaled and embedded tsnet nodes.
- Reconcile directed observations into logical traffic edges.
- Distinguish direct, DERP, peer relay, and unknown paths.
- Calculate traffic rates from cumulative counter deltas.
- Preserve path events and observation provenance.
- Render a stable live graph and edge details.
- Add Peer Relay, tsnet, and tsbridge observers through one protocol.
- Optionally enrich display metadata and search with a read-only control-plane
  device directory kept separate from runtime observations.

## Non-goals

Tailpath is not an ACL visualizer, admin console, packet capture tool, network
manager, active connectivity tester, or full metrics platform. Mobile clients
do not require a Tailpath agent.

## Product boundary

The first stable release targets one self-hosted server, one Tailnet, 250 known
nodes, and 1,000 visible active/recent edges. Missing observations stay unknown;
Tailpath does not infer an unobserved data path.

A directory device is not evidence that the device is online, observable, or
communicating. Live remains a runtime data-plane view. The Devices workspace is
an optional control-plane catalog whose connected-to-control status is shown as
a separate dimension from Tailpath runtime evidence.
