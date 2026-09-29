import type { PathObservation } from "../api/types";

export function pathIdentityKey(path: PathObservation): string {
  switch (path.kind) {
    case "direct":
      return "direct";
    case "derp":
      return `derp:${path.derpRegion?.trim().toLowerCase() || "unknown"}`;
    case "peer_relay":
      return [
        "peer-relay",
        `stable:${path.peerRelayStableNodeId?.trim() || "none"}`,
        `endpoint:${path.peerRelayEndpoint?.trim() || "none"}`,
        `vni:${path.peerRelayVni ?? "none"}`,
        `resolution:${path.peerRelayResolution ?? "none"}`,
      ].join(":");
    default:
      return "unknown";
  }
}
