# Deployment

[中文版](deployment.zh-CN.md)

The default Compose service runs `tailpath server --network=tsnet`, stores tsnet
state and SQLite under `/var/lib/tailpath`, and joins the Tailnet with
`TS_AUTHKEY` on first start. Remove the key after enrollment.

Linux collectors can run natively or through the optional host-network Compose
profile with the tailscaled LocalAPI socket mounted read-only. macOS and Windows
collectors run as native binaries.

The server identity must be dedicated to Tailpath. A tailscaled-mode deployment
that shares an identity must explicitly acknowledge that all traffic to that
node is hidden as system telemetry.

Production operators should back up the SQLite file with the documented online
backup command, persist tsnet state, and upgrade one version at a time. The
server runs migrations before readiness.
