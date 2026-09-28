import type { ElementDefinition } from "cytoscape";
import type {
  PathCandidate,
  PathKind,
  PathObservation,
  Topology,
  TopologyEdge,
  TopologyNode,
} from "../api/types";
import { formatCompactRate, nodeLabel } from "./format";
import { identityPresentation, unresolvedNodeLabel } from "./identity";
import { platformPresentation } from "./platform";

export const minimumEdgeCenterDistance = 220;
export const minimumTrafficWidth = 1.5;
export const maximumTrafficWidth = 5.5;
export const trafficVisualFloor = 1024;
export const trafficVisualCeiling = 100 * 1024 * 1024;
const edgeEndpointAndLabelPadding = 160;
const edgeLabelCharacterWidth = 5;

export type PathFilter = "all" | PathKind;
export type EmptyTrafficReason = "no-active" | "no-recent" | "no-match";

interface BuildOptions {
  pathFilter: PathFilter;
  showRecent: boolean;
  query: string;
}

export function buildElements(
  topology: Topology,
  options: BuildOptions,
): ElementDefinition[] {
  const visibleEdges = topology.edges.filter((edge) =>
    edgeIsVisible(edge, options.pathFilter, options.showRecent),
  );
  const nodeMap = new Map(topology.nodes.map((node) => [node.id, node]));
  const stableNodeMap = new Map(
    topology.nodes.flatMap((node) =>
      node.stableNodeId ? [[node.stableNodeId, node] as const] : [],
    ),
  );
  const intermediates = new Map(
    visibleEdges.map((edge) => [
      edge.id,
      intermediatesFor(edge, stableNodeMap),
    ]),
  );
  const relevantNodeIDs = visibleTopologyNodeIDs(
    topology,
    options.pathFilter,
    options.showRecent,
  );
  const peerRelayNodeIDs = new Set(
    [...intermediates.values()]
      .flatMap((values) => values.map((intermediate) => intermediate.nodeID))
      .filter((id): id is string => Boolean(id)),
  );
  const elements: ElementDefinition[] = topology.nodes
    .filter((node) => relevantNodeIDs.has(node.id))
    .map((node) =>
      nodeElement(node, options.query, peerRelayNodeIDs.has(node.id)),
    );
  const virtualNodes = new Set<string>();
  const activeIntermediateIDs = new Set(
    visibleEdges
      .filter((edge) => edge.state === "active")
      .flatMap((edge) =>
        (intermediates.get(edge.id) ?? []).map(
          (intermediate) => intermediate.id,
        ),
      )
      .filter((id): id is string => Boolean(id)),
  );

  for (const edge of visibleEdges) {
    const edgeIntermediates = intermediates.get(edge.id) ?? [];
    if (!edgeIntermediates.length) {
      elements.push(edgeElement(edge, edge.source, edge.target, "main", true));
      continue;
    }
    edgeIntermediates.forEach((intermediate, index) => {
      if (!nodeMap.has(intermediate.id) && !virtualNodes.has(intermediate.id)) {
        elements.push({
          group: "nodes",
          data: {
            id: intermediate.id,
            label: intermediate.label,
            kind: intermediate.kind,
            logicalEdgeId: intermediate.logicalEdgeId,
            candidateState: intermediate.candidateState,
          },
          classes: `${intermediate.classes} ${
            activeIntermediateIDs.has(intermediate.id) ? "active" : "recent"
          }`,
        });
        virtualNodes.add(intermediate.id);
      }
      const candidateClass =
        edgeIntermediates.length > 1
          ? `switching-candidate ${intermediate.candidateState}`
          : "";
      elements.push(
        edgeElement(
          edge,
          edge.source,
          intermediate.id,
          `candidate-${index}-source`,
          index === 0,
          candidateClass,
        ),
      );
      elements.push(
        edgeElement(
          edge,
          intermediate.id,
          edge.target,
          `candidate-${index}-target`,
          false,
          candidateClass,
        ),
      );
    });
  }
  return elements;
}

export function visibleTopologyNodeIDs(
  topology: Topology,
  pathFilter: PathFilter,
  showRecent: boolean,
): Set<string> {
  const visibleEdges = topology.edges.filter((edge) =>
    edgeIsVisible(edge, pathFilter, showRecent),
  );
  const result = new Set(
    visibleEdges.flatMap((edge) => [edge.source, edge.target]),
  );
  const nodesByStableID = new Map(
    topology.nodes.flatMap((node) =>
      node.stableNodeId ? [[node.stableNodeId, node] as const] : [],
    ),
  );
  for (const edge of visibleEdges) {
    for (const intermediate of intermediatesFor(edge, nodesByStableID)) {
      if (intermediate.nodeID) result.add(intermediate.nodeID);
    }
  }
  return result;
}

function nodeElement(
  node: TopologyNode,
  query: string,
  isPeerRelay: boolean,
): ElementDefinition {
  const label = unresolvedNodeLabel(node.identityStatus) ?? nodeLabel(node);
  const platform = platformPresentation(node.os);
  const iconLayers = nodeIconLayers(node, platform.asset, isPeerRelay);
  const matches =
    !query ||
    `${label} ${node.dnsName ?? ""} ${node.os ?? ""} ${node.tailscaleIps?.join(" ") ?? ""}`
      .toLowerCase()
      .includes(query.toLowerCase());
  return {
    group: "nodes",
    data: {
      id: node.id,
      label,
      kind: isPeerRelay ? "peer-relay" : "tailnet",
      observable: node.observable,
      online: node.online,
      dimmed: !matches,
      os: node.os ?? "",
      persistable: true,
      identityStatus: node.identityStatus ?? "",
      ...iconLayers,
    },
    classes: [
      isPeerRelay ? "relay-node peer-relay" : "device-node",
      node.observable ? "runtime-telemetry" : "peer-only",
      node.observable && !node.online ? "offline" : "",
      node.clockSkewed ? "clock-skewed" : "",
      node.identityStatus ? `identity-${node.identityStatus}` : "",
      matches ? "" : "dimmed",
    ].join(" "),
  };
}

function nodeIconLayers(
  node: TopologyNode,
  platformIcon: string,
  isPeerRelay: boolean,
) {
  const images: string[] = [];
  const widths: string[] = [];
  const heights: string[] = [];
  const positionsX: string[] = [];
  const positionsY: string[] = [];
  const add = (image: string, size: string, x: string, y: string) => {
    images.push(image);
    widths.push(size);
    heights.push(size);
    positionsX.push(x);
    positionsY.push(y);
  };
  const identity = identityPresentation(node.identityStatus);
  add(
    node.identityStatus && node.identityStatus !== "resolved" && identity
      ? identity.asset
      : platformIcon,
    isPeerRelay ? "20px" : "24px",
    "50%",
    isPeerRelay ? "50%" : "46%",
  );
  if (node.observable) {
    add(
      "/runtime-telemetry.svg",
      isPeerRelay ? "14px" : "15px",
      isPeerRelay ? "88%" : "84%",
      isPeerRelay ? "12%" : "16%",
    );
  }
  if (node.clockSkewed) {
    add(
      "/clock-skew.svg",
      isPeerRelay ? "15px" : "16px",
      isPeerRelay ? "88%" : "84%",
      isPeerRelay ? "88%" : "84%",
    );
  }
  return images.length
    ? {
        backgroundImages: images,
        backgroundWidths: widths,
        backgroundHeights: heights,
        backgroundPositionsX: positionsX,
        backgroundPositionsY: positionsY,
      }
    : {};
}

interface PathIntermediate {
  id: string;
  label: string;
  kind: string;
  classes: string;
  logicalEdgeId?: string;
  nodeID?: string;
  candidateState?: "identified" | "pending";
}

function intermediatesFor(
  edge: TopologyEdge,
  nodesByStableID: Map<string, TopologyNode>,
): PathIntermediate[] {
  if (edge.path.kind === "derp") {
    const region = edge.path.derpRegion || "unknown";
    return [
      {
        id: `derp:${region}`,
        label: `DERP ${region}`,
        kind: "derp",
        classes: "relay-node derp",
      },
    ];
  }
  if (edge.path.kind === "peer_relay") {
    return peerRelayCandidates(edge).map((candidate) => {
      const stableID = candidate.path.peerRelayStableNodeId;
      const node = stableID ? nodesByStableID.get(stableID) : undefined;
      const key = peerRelayCandidateKey(candidate.path);
      const candidateState = stableID ? "identified" : "pending";
      return {
        id:
          node?.id ||
          (stableID
            ? `peer-relay:${stableID}`
            : `peer-relay:${edge.id}:${encodeURIComponent(key)}`),
        label: node ? nodeLabel(node) : "Peer Relay",
        kind: "peer-relay",
        classes: `relay-node peer-relay candidate-${candidateState}`,
        logicalEdgeId: node ? undefined : edge.id,
        nodeID: node?.id,
        candidateState,
      };
    });
  }
  if (edge.path.kind === "unknown") {
    return [
      {
        id: `unknown-marker:${edge.id}`,
        label: "?",
        kind: "unknown",
        classes: "path-marker unknown-marker",
        logicalEdgeId: edge.id,
      },
    ];
  }
  return [];
}

export function peerRelayCandidates(edge: TopologyEdge): PathCandidate[] {
  const supplied = edge.pathCandidates?.filter(
    (candidate) => candidate.path.kind === "peer_relay",
  );
  if (supplied?.length) return supplied;
  const paths = [edge.path, ...(edge.conflicts ?? [])].filter(
    (path) => path.kind === "peer_relay",
  );
  const seen = new Set<string>();
  return paths.flatMap((path) => {
    const key = peerRelayCandidateKey(path);
    if (seen.has(key)) return [];
    seen.add(key);
    const matching = edge.observations.filter(
      (observation) => peerRelayCandidateKey(observation.path) === key,
    );
    const lastObservedAt = matching.reduce(
      (latest, observation) =>
        observation.receivedAt > latest ? observation.receivedAt : latest,
      edge.lastActive,
    );
    return [{ path, lastObservedAt, observerCount: matching.length }];
  });
}

export function peerRelayCandidateKey(path: PathObservation): string {
  if (path.peerRelayStableNodeId) return `node:${path.peerRelayStableNodeId}`;
  if (path.peerRelayEndpoint) return `endpoint:${path.peerRelayEndpoint}`;
  if (path.peerRelayVni !== undefined) return `vni:${path.peerRelayVni}`;
  return "unknown";
}

function edgeElement(
  edge: TopologyEdge,
  source: string,
  target: string,
  segment: string,
  showLabel: boolean,
  candidateClass = "",
): ElementDefinition {
  const totalRate = edge.aToBBytesPerSecond + edge.bToABytesPerSecond;
  const isActive = edge.state === "active";
  const label = isActive && showLabel ? formatCompactRate(totalRate) : "";
  return {
    group: "edges",
    data: {
      id: `${edge.id}:${segment}`,
      source,
      target,
      logicalEdgeId: edge.id,
      label,
      idealLength: edgeIdealLength(label),
      trafficWidth: isActive ? trafficWidth(totalRate) : minimumTrafficWidth,
    },
    classes: [
      edge.path.kind,
      edge.state,
      candidateClass,
      isActive && edge.aToBBytesPerSecond > 0 ? "flow-forward" : "",
      isActive && edge.bToABytesPerSecond > 0 ? "flow-reverse" : "",
    ].join(" "),
  };
}

export function edgeIdealLength(label: string): number {
  return edgeIdealLengthForWidth(label.length * edgeLabelCharacterWidth);
}

export function edgeIdealLengthForWidth(labelWidth: number): number {
  return Math.max(
    minimumEdgeCenterDistance,
    edgeEndpointAndLabelPadding + labelWidth,
  );
}

export function trafficWidth(bytesPerSecond: number): number {
  if (bytesPerSecond <= trafficVisualFloor) return minimumTrafficWidth;
  if (bytesPerSecond >= trafficVisualCeiling) return maximumTrafficWidth;
  const range = Math.log(trafficVisualCeiling / trafficVisualFloor);
  const ratio = Math.log(bytesPerSecond / trafficVisualFloor) / range;
  const width =
    minimumTrafficWidth + ratio * (maximumTrafficWidth - minimumTrafficWidth);
  return Math.round(width * 4) / 4;
}

export function edgeIsVisible(
  edge: TopologyEdge,
  pathFilter: PathFilter,
  showRecent: boolean,
): boolean {
  return (
    !edge.systemTelemetry &&
    (pathFilter === "all" || edge.path.kind === pathFilter) &&
    (showRecent || edge.state === "active")
  );
}

export function emptyTrafficReason(
  edges: TopologyEdge[],
  pathFilter: PathFilter,
  showRecent: boolean,
): EmptyTrafficReason | null {
  const userTrafficEdges = edges.filter((edge) => !edge.systemTelemetry);
  if (
    userTrafficEdges.some((edge) => edgeIsVisible(edge, pathFilter, showRecent))
  ) {
    return null;
  }
  if (
    userTrafficEdges.length > 0 &&
    pathFilter !== "all" &&
    !userTrafficEdges.some((edge) => edge.path.kind === pathFilter)
  ) {
    return "no-match";
  }
  return showRecent ? "no-recent" : "no-active";
}
