import type { ElementDefinition } from "cytoscape";
import type {
  PathKind,
  Topology,
  TopologyEdge,
  TopologyNode,
} from "../api/types";
import { formatCompactRate, nodeLabel } from "./format";

export const minimumEdgeCenterDistance = 160;
export const minimumTrafficWidth = 1.5;
export const maximumTrafficWidth = 5.5;
export const trafficVisualFloor = 1024;
export const trafficVisualCeiling = 100 * 1024 * 1024;
const edgeEndpointAndLabelPadding = 124;
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
  const relevantNodeIDs = new Set(
    visibleEdges.flatMap((edge) => [edge.source, edge.target]),
  );
  for (const edge of visibleEdges) {
    const intermediate = intermediateFor(edge, topology.nodes);
    if (intermediate?.nodeID) relevantNodeIDs.add(intermediate.nodeID);
  }
  const elements: ElementDefinition[] = topology.nodes
    .filter((node) => relevantNodeIDs.has(node.id))
    .map((node) => nodeElement(node, options.query));
  const virtualNodes = new Set<string>();
  const activeIntermediateIDs = new Set(
    visibleEdges
      .filter((edge) => edge.state === "active")
      .map((edge) => intermediateFor(edge, topology.nodes)?.id)
      .filter((id): id is string => Boolean(id)),
  );

  for (const edge of visibleEdges) {
    const intermediate = intermediateFor(edge, topology.nodes);
    if (!intermediate) {
      elements.push(edgeElement(edge, edge.source, edge.target, "main"));
      continue;
    }
    if (!nodeMap.has(intermediate.id) && !virtualNodes.has(intermediate.id)) {
      elements.push({
        group: "nodes",
        data: {
          id: intermediate.id,
          label: intermediate.label,
          kind: intermediate.kind,
          logicalEdgeId: intermediate.logicalEdgeId,
        },
        classes: `${intermediate.classes} ${
          activeIntermediateIDs.has(intermediate.id) ? "active" : "recent"
        }`,
      });
      virtualNodes.add(intermediate.id);
    }
    elements.push(edgeElement(edge, edge.source, intermediate.id, "source"));
    elements.push(edgeElement(edge, intermediate.id, edge.target, "target"));
  }
  return elements;
}

function nodeElement(node: TopologyNode, query: string): ElementDefinition {
  const label = nodeLabel(node);
  const matches =
    !query ||
    `${label} ${node.dnsName ?? ""} ${node.tailscaleIps?.join(" ") ?? ""}`
      .toLowerCase()
      .includes(query.toLowerCase());
  return {
    group: "nodes",
    data: {
      id: node.id,
      label,
      kind: "tailnet",
      observable: node.observable,
      online: node.online,
      dimmed: !matches,
    },
    classes: [
      node.observable ? "runtime-telemetry" : "peer-only",
      node.observable && !node.online ? "offline" : "",
      node.clockSkewed ? "clock-skewed" : "",
      matches ? "" : "dimmed",
    ].join(" "),
  };
}

function intermediateFor(edge: TopologyEdge, nodes: TopologyNode[]) {
  if (edge.path.kind === "derp") {
    const region = edge.path.derpRegion || "unknown";
    return {
      id: `derp:${region}`,
      label: `DERP ${region}`,
      kind: "derp",
      classes: "relay-node derp",
    };
  }
  if (edge.path.kind === "peer_relay") {
    const stableID = edge.path.peerRelayStableNodeId;
    const node = nodes.find((candidate) => candidate.stableNodeId === stableID);
    return {
      id: node?.id || `peer-relay:${stableID || "unknown"}`,
      label: node ? nodeLabel(node) : "Peer Relay",
      kind: "peer-relay",
      classes: "relay-node peer-relay",
      nodeID: node?.id,
    };
  }
  if (edge.path.kind === "unknown") {
    return {
      id: `unknown-marker:${edge.id}`,
      label: "?",
      kind: "unknown",
      classes: "path-marker unknown-marker",
      logicalEdgeId: edge.id,
    };
  }
  return null;
}

function edgeElement(
  edge: TopologyEdge,
  source: string,
  target: string,
  segment: string,
): ElementDefinition {
  const totalRate = edge.aToBBytesPerSecond + edge.bToABytesPerSecond;
  const isActive = edge.state === "active";
  const label =
    isActive && segment !== "target" ? formatCompactRate(totalRate) : "";
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
    (pathFilter === "all" || edge.path.kind === pathFilter) &&
    (showRecent || edge.state === "active")
  );
}

export function emptyTrafficReason(
  edges: TopologyEdge[],
  pathFilter: PathFilter,
  showRecent: boolean,
): EmptyTrafficReason | null {
  if (edges.some((edge) => edgeIsVisible(edge, pathFilter, showRecent))) {
    return null;
  }
  if (
    edges.length > 0 &&
    pathFilter !== "all" &&
    !edges.some((edge) => edge.path.kind === pathFilter)
  ) {
    return "no-match";
  }
  return showRecent ? "no-recent" : "no-active";
}
