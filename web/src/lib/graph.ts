import type { ElementDefinition } from "cytoscape";
import type {
  DirectionalPathState,
  PathCandidate,
  PathKind,
  PathObservation,
  Topology,
  TopologyEdge,
  TopologyNode,
} from "../api/types";
import {
  formatCompactRate,
  nodeLabel,
  unresolvedPeerRelayLabel,
} from "./format";
import { identityPresentation, unresolvedNodeLabel } from "./identity";
import { platformPresentation } from "./platform";
import { pathIdentityKey } from "./pathIdentity";

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

interface RenderedRoute {
  id: string;
  from: string;
  to: string;
  path: PathObservation;
  rate: number;
  labelRate?: number;
  showLabel: boolean;
  directional: boolean;
  fallback: boolean;
  curveClass?: "route-a" | "route-b";
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
  const routes = new Map(
    visibleEdges.map((edge) => [edge.id, routesFor(edge)]),
  );
  const intermediates = new Map(
    visibleEdges.map((edge) => [
      edge.id,
      (routes.get(edge.id) ?? []).flatMap((route) =>
        intermediatesForRoute(edge, route, stableNodeMap),
      ),
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
    for (const route of routes.get(edge.id) ?? []) {
      const edgeIntermediates = intermediatesForRoute(
        edge,
        route,
        stableNodeMap,
      );
      if (!edgeIntermediates.length) {
        const segment = edge.directions?.length ? `${route.id}-main` : "main";
        elements.push(
          routeEdgeElement(
            edge,
            route,
            route.from,
            route.to,
            segment,
            route.showLabel,
          ),
        );
        continue;
      }
      edgeIntermediates.forEach((intermediate, index) => {
        if (
          !nodeMap.has(intermediate.id) &&
          !virtualNodes.has(intermediate.id)
        ) {
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
          !edge.directions?.length && edgeIntermediates.length > 1
            ? `switching-candidate ${intermediate.candidateState}`
            : "";
        const segmentPrefix = edge.directions?.length
          ? `${route.id}-${index}`
          : `candidate-${index}`;
        elements.push(
          routeEdgeElement(
            edge,
            route,
            route.from,
            intermediate.id,
            `${segmentPrefix}-source`,
            route.showLabel && index === 0,
            candidateClass,
          ),
        );
        elements.push(
          routeEdgeElement(
            edge,
            route,
            intermediate.id,
            route.to,
            `${segmentPrefix}-target`,
            false,
            candidateClass,
          ),
        );
      });
    }
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
    for (const route of routesFor(edge)) {
      for (const intermediate of intermediatesForRoute(
        edge,
        route,
        nodesByStableID,
      )) {
        if (intermediate.nodeID) result.add(intermediate.nodeID);
      }
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

function intermediatesForRoute(
  edge: TopologyEdge,
  route: RenderedRoute,
  nodesByStableID: Map<string, TopologyNode>,
): PathIntermediate[] {
  const path = route.path;
  const routeSuffix = route.directional || route.fallback ? `:${route.id}` : "";
  if (path.kind === "derp") {
    const region = path.derpRegion?.trim() || "unknown";
    // A DERP region is a shared intermediate, even when opposite directions
    // use it through separate rendered routes. Direction belongs to the
    // parallel edges; putting the route ID in the node ID invents duplicate
    // DERP nodes for one relay.
    const id = pathIdentityKey(path);
    return [
      {
        id,
        label: `DERP ${region}`,
        kind: "derp",
        classes: "relay-node derp",
      },
    ];
  }
  if (path.kind === "peer_relay") {
    const candidates = edge.directions?.length
      ? [{ path, lastObservedAt: edge.lastActive, observerCount: 1 }]
      : peerRelayCandidates(edge);
    return candidates.map((candidate) => {
      const stableID = candidate.path.peerRelayStableNodeId;
      const node = stableID ? nodesByStableID.get(stableID) : undefined;
      const key = peerRelayCandidateKey(candidate.path);
      const candidateState = stableID ? "identified" : "pending";
      return {
        id:
          node?.id ||
          (stableID
            ? `peer-relay:${stableID}`
            : `peer-relay:${edge.id}:${encodeURIComponent(key)}${routeSuffix}`),
        label: node
          ? nodeLabel(node)
          : unresolvedPeerRelayLabel(candidate.path),
        kind: "peer-relay",
        classes: `relay-node peer-relay candidate-${candidateState}`,
        logicalEdgeId: node ? undefined : edge.id,
        nodeID: node?.id,
        candidateState,
      };
    });
  }
  if (path.kind === "unknown") {
    return [
      {
        id: `unknown-marker:${edge.id}${routeSuffix}`,
        label: "?",
        kind: "unknown",
        classes: "path-marker unknown-marker",
        logicalEdgeId: edge.id,
      },
    ];
  }
  return [];
}

export function edgeDirections(edge: TopologyEdge): DirectionalPathState[] {
  return [...(edge.directions ?? [])].sort((left, right) => {
    if (left.fromNodeId === edge.source && right.fromNodeId !== edge.source)
      return -1;
    if (right.fromNodeId === edge.source && left.fromNodeId !== edge.source)
      return 1;
    return `${left.fromNodeId}:${left.toNodeId}`.localeCompare(
      `${right.fromNodeId}:${right.toNodeId}`,
    );
  });
}

export function edgeIsAsymmetric(edge: TopologyEdge): boolean {
  const directions = edgeDirections(edge);
  return (
    directions.length === 2 &&
    directionPathKey(directions[0]) !== directionPathKey(directions[1])
  );
}

export function edgePathKinds(edge: TopologyEdge): Set<PathKind> {
  const directions = edgeDirections(edge);
  if (!directions.length) return new Set([edge.path.kind]);
  return new Set(
    directions.flatMap((direction) => [
      direction.primaryPath.kind,
      ...(direction.fallbackPath ? [direction.fallbackPath.kind] : []),
    ]),
  );
}

function directionPathKey(direction: DirectionalPathState): string {
  return `${pathIdentityKey(direction.primaryPath)}|${
    direction.fallbackPath ? pathIdentityKey(direction.fallbackPath) : "none"
  }`;
}

function routesFor(edge: TopologyEdge): RenderedRoute[] {
  const directions = edgeDirections(edge);
  if (!directions.length) {
    return [
      {
        id: "legacy",
        from: edge.source,
        to: edge.target,
        path: edge.path,
        rate: edge.aToBBytesPerSecond + edge.bToABytesPerSecond,
        showLabel: true,
        directional: false,
        fallback: false,
      },
    ];
  }
  const asymmetric = edgeIsAsymmetric(edge);
  if (!asymmetric) {
    const direction = directions[0];
    const fromSource = direction.fromNodeId === edge.source;
    const directionRate = fromSource
      ? edge.aToBBytesPerSecond
      : edge.bToABytesPerSecond;
    const routes: RenderedRoute[] = [
      {
        id: "combined-primary",
        from: direction.fromNodeId,
        to: direction.toNodeId,
        path: direction.primaryPath,
        rate:
          directions.length === 1
            ? directionRate
            : edge.aToBBytesPerSecond + edge.bToABytesPerSecond,
        labelRate: edge.aToBBytesPerSecond + edge.bToABytesPerSecond,
        showLabel: true,
        directional: directions.length === 1,
        fallback: false,
      },
    ];
    if (direction.fallbackPath) {
      routes.push({
        ...routes[0],
        id: "combined-fallback",
        path: direction.fallbackPath,
        rate: 0,
        showLabel: false,
        fallback: true,
      });
    }
    return routes;
  }
  return directions.flatMap((direction, index) => {
    const fromSource = direction.fromNodeId === edge.source;
    const primary: RenderedRoute = {
      id: `direction-${index}-primary`,
      from: direction.fromNodeId,
      to: direction.toNodeId,
      path: direction.primaryPath,
      rate: fromSource ? edge.aToBBytesPerSecond : edge.bToABytesPerSecond,
      labelRate: edge.aToBBytesPerSecond + edge.bToABytesPerSecond,
      showLabel: index === 0,
      directional: true,
      fallback: false,
      curveClass: index === 0 ? "route-a" : "route-b",
    };
    return direction.fallbackPath
      ? [
          primary,
          {
            ...primary,
            id: `direction-${index}-fallback`,
            path: direction.fallbackPath,
            rate: 0,
            showLabel: false,
            fallback: true,
          },
        ]
      : [primary];
  });
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

function routeEdgeElement(
  edge: TopologyEdge,
  route: RenderedRoute,
  source: string,
  target: string,
  segment: string,
  showLabel: boolean,
  candidateClass = "",
): ElementDefinition {
  const isActive = edge.state === "active";
  const hasForwardFlow = route.directional
    ? route.rate > 0
    : edge.aToBBytesPerSecond > 0;
  const label =
    isActive && showLabel
      ? formatCompactRate(route.labelRate ?? route.rate)
      : "";
  return {
    group: "edges",
    data: {
      id: `${edge.id}:${segment}`,
      source,
      target,
      logicalEdgeId: edge.id,
      label,
      idealLength: edgeIdealLength(label),
      trafficWidth:
        isActive && !route.fallback
          ? trafficWidth(route.rate)
          : minimumTrafficWidth,
    },
    classes: [
      route.path.kind,
      edge.state,
      candidateClass,
      route.directional ? "directional-route" : "",
      route.curveClass ?? "",
      route.fallback ? "fallback-route" : "",
      isActive && hasForwardFlow ? "flow-forward" : "",
      isActive && !route.directional && edge.bToABytesPerSecond > 0
        ? "flow-reverse"
        : "",
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
    (pathFilter === "all" || edgePathKinds(edge).has(pathFilter)) &&
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
    !userTrafficEdges.some((edge) => edgePathKinds(edge).has(pathFilter))
  ) {
    return "no-match";
  }
  return showRecent ? "no-recent" : "no-active";
}
