# Issue 200: Do not promote unconfirmed runtime paths

Issue: https://github.com/GhostFlying/tailpath/issues/200
Baseline: origin/main 2068f324f2da4eeec534b69864f7722540fa2fcb.

## Investigation

Pinned tailscale.com v1.102.2 populates PeerStatus.Relay from the peer's DERP address even before any outbound send. Active records recent sends, not successful handshakes. PeerStatusLite documents a zero LastHandshake as no successful handshake/key confirmation since the peer was last known to WireGuard; idle peer eviction can reset this evidence. WireGuard SendBuffers increments TX after a successful local bind.Send, including handshake sends; this is not a delivery acknowledgement. CurAddr/PeerRelay describe selected send endpoints rather than remote application delivery.

Sources: tailscale.com/ipn/ipnstate/ipnstate.go, wgengine/magicsock/endpoint.go (populatePeerStatus), wgengine/userspace.go (getPeerStatusLite), and pinned wireguard-go/device/peer.go (SendBuffers). Native and embedded sources share internal/tailscalestatus; current main also has a bounded Peer Relay fallback tracker, which must not retain inference across loss of confirmation.

## Implementation and acceptance

1. Normalize any path without a successful current-runtime handshake to Unknown before path precedence/fallback classification. Preserve identity and raw RX/TX counters. Keep successful-session Direct > Peer Relay/DERP precedence as currently implemented; no invented recency timeout or delivery guarantee.
2. Test the observed TX-only/home-DERP/Active=true case, unconfirmed Direct and Peer Relay candidates, confirmed route precedence, handshake acquisition/loss, fallback reset, and embedded/native adapter behavior.
3. Update English/Chinese protocol and architecture documentation to state the passive evidence boundary and counter limits. No API/schema migration; old history cannot be reclassified without missing handshake evidence. Old collectors must be upgraded to gain this behavior.
4. Verify the normalized Unknown output stays Unknown in Live/History at desktop/mobile sizes without creating a DERP intermediate node. Run focused regression tests and canonical dev-container make check. Commit atomic fix/docs/regressions and open a draft PR; ready only after checks/screenshots.

## Boundaries

No active probes, packet capture, network/grant/config changes, production deployment, or unrelated user-document edits. A historical handshake permits interpreting the selected runtime path but does not prove current reachability or delivered application bytes. Unknown is deliberately conservative when WireGuard drops idle state. Preserve raw counters instead of relabeling all TX as application traffic.

## Current state / next step

Investigation complete; implementation pending. Next: shared normalization gate and regressions, followed by browser verification and PR.

## Verification

Pending.
