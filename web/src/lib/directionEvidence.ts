import type { TopologyEdge } from "../api/types";

type Observation = TopologyEdge["observations"][number];

export interface DirectionCoverage {
  point: "sender" | "receiver" | "none";
  observation?: Observation;
}

export const inboundPathLimit =
  "Tailscale status does not expose per-peer inbound paths.";

// Coverage describes available endpoint reports, not the route used by packets.
// A receiver's outbound path must never be copied into the reverse direction.
export function directionCoverage(
  from: string,
  to: string,
  observations: Observation[],
): DirectionCoverage {
  const sender = observations.find(
    (observation) =>
      observation.observerId === from && !observation.relaySession,
  );
  if (sender) return { point: "sender", observation: sender };
  const receiver = observations.find(
    (observation) => observation.observerId === to && !observation.relaySession,
  );
  return receiver
    ? { point: "receiver", observation: receiver }
    : { point: "none" };
}

export function unknownPathReason(
  coverage: DirectionCoverage,
  historical = false,
): string {
  switch (coverage.point) {
    case "receiver":
      return historical
        ? "No sender path observation retained."
        : "No sender path observation. " + inboundPathLimit;
    case "sender":
      return "The sender reported no usable path.";
    default:
      return historical
        ? "No path observation retained for this direction."
        : "No fresh path observation for this direction.";
  }
}

// Match the aggregator's ten-second traffic window and sender-first counter
// precedence. Use the server snapshot time and ReceivedAt, never collector clocks.
export function liveTrafficObservation(
  from: string,
  to: string,
  observations: Observation[],
  generatedAt: string,
): { point: "sender" | "receiver" | "relay"; observation: Observation } | null {
  const at = Date.parse(generatedAt);
  const current = observations.filter((observation) => {
    const age = at - Date.parse(observation.receivedAt);
    return age >= 0 && age <= 10_000;
  });
  const coverage = directionCoverage(from, to, current);
  if (coverage.point !== "none" && coverage.observation) {
    return { point: coverage.point, observation: coverage.observation };
  }
  const relay = current
    .filter(
      (observation) =>
        observation.observerId !== from &&
        observation.observerId !== to &&
        observation.path.kind === "peer_relay",
    )
    .sort(
      (left, right) =>
        Date.parse(right.receivedAt) - Date.parse(left.receivedAt),
    )[0];
  return relay ? { point: "relay", observation: relay } : null;
}
