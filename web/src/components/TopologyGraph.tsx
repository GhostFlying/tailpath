import { useEffect, useMemo, useRef } from "react";
import { Maximize2, RefreshCcw } from "lucide-react";
import cytoscape, {
  type Core,
  type CollectionReturnValue,
  type ElementDefinition,
  type NodeSingular,
  type StylesheetCSS,
} from "cytoscape";
import type { Topology } from "../api/types";
import {
  buildElements,
  edgeIdealLengthForWidth,
  type PathFilter,
} from "../lib/graph";
import {
  clearLayoutCache,
  readLayoutCache,
  type LayoutPosition,
  writeLayoutCache,
} from "../lib/layoutCache";

interface Props {
  topology: Topology;
  pathFilter: PathFilter;
  showRecent: boolean;
  query: string;
  selectedEdgeId: string | null;
  onSelectEdge: (edgeId: string | null) => void;
  onSelectNode: (nodeId: string | null) => void;
}

const automaticCoseNodeLimit = 100;

const styles: StylesheetCSS[] = [
  {
    selector: "node",
    css: {
      width: 52,
      height: 52,
      "background-color": "#ffffff",
      "border-width": 2,
      "border-color": "#5f6b73",
      label: "data(label)",
      color: "#1c252b",
      "font-family": "Inter, ui-sans-serif, system-ui, sans-serif",
      "font-size": 12,
      "font-weight": 600,
      "text-valign": "bottom",
      "text-margin-y": 10,
      "text-background-color": "#f6f7f8",
      "text-background-opacity": 0.88,
      "text-background-padding": "3px",
    },
  },
  {
    selector: "node[backgroundImages]",
    css: {
      "background-image": "data(backgroundImages)",
      "background-width": "data(backgroundWidths)",
      "background-height": "data(backgroundHeights)",
      "background-position-x": "data(backgroundPositionsX)",
      "background-position-y": "data(backgroundPositionsY)",
      "background-image-containment": "over",
    },
  },
  { selector: "node.offline", css: { opacity: 0.42 } },
  { selector: "node.dimmed", css: { opacity: 0.18 } },
  {
    selector: "node.relay-node",
    css: {
      shape: "round-rectangle",
      width: 66,
      height: 34,
      "font-size": 10,
      "text-valign": "center",
      "text-margin-y": 0,
      "border-width": 1,
    },
  },
  {
    selector: "node.derp",
    css: { "background-color": "#fff2cf", "border-color": "#b57900" },
  },
  {
    selector: "node.peer-relay",
    css: {
      shape: "ellipse",
      width: 44,
      height: 44,
      "background-color": "#f7e8f4",
      "border-color": "#a4488e",
      "text-valign": "bottom",
      "text-margin-y": 9,
    },
  },
  {
    selector: "node.path-marker",
    css: {
      width: 18,
      height: 18,
      "font-size": 10,
      "font-weight": 700,
      "text-valign": "center",
      "text-margin-y": 0,
      "text-background-opacity": 0,
      "border-width": 1.5,
    },
  },
  {
    selector: "node.unknown-marker",
    css: { "background-color": "#ffffff", "border-color": "#7f8a91" },
  },
  {
    selector: "node.relay-node.recent, node.path-marker.recent",
    css: { opacity: 0.5 },
  },
  {
    selector: "edge",
    css: {
      width: "data(trafficWidth)",
      "curve-style": "bezier",
      "line-color": "#8b969d",
      "source-arrow-shape": "none",
      "target-arrow-shape": "none",
      "source-arrow-color": "#8b969d",
      "target-arrow-color": "#8b969d",
      "arrow-scale": 0.7,
      label: "data(label)",
      "font-size": 8,
      color: "#566168",
      "text-background-color": "#f6f7f8",
      "text-background-opacity": 0.93,
      "text-background-padding": "3px",
      "text-rotation": "none",
      "text-wrap": "ellipsis",
      "text-max-width": "80px",
    },
  },
  {
    selector: "edge.flow-forward",
    css: { "target-arrow-shape": "triangle" },
  },
  {
    selector: "edge.flow-reverse",
    css: { "source-arrow-shape": "triangle" },
  },
  {
    selector: "edge.direct",
    css: {
      "line-color": "#16877a",
      "source-arrow-color": "#16877a",
      "target-arrow-color": "#16877a",
    },
  },
  {
    selector: "edge.derp",
    css: {
      "line-color": "#bd7b00",
      "source-arrow-color": "#bd7b00",
      "target-arrow-color": "#bd7b00",
    },
  },
  {
    selector: "edge.peer_relay",
    css: {
      "line-color": "#a4488e",
      "source-arrow-color": "#a4488e",
      "target-arrow-color": "#a4488e",
    },
  },
  {
    selector: "edge.unknown",
    css: {
      "line-color": "#7f8a91",
      "source-arrow-color": "#7f8a91",
      "target-arrow-color": "#7f8a91",
    },
  },
  {
    selector: "edge.recent",
    css: {
      "line-style": "dashed",
      "line-opacity": 0.5,
      "text-opacity": 0,
    },
  },
  {
    selector: "edge:selected",
    css: {
      "overlay-color": "#1c252b",
      "overlay-opacity": 0.08,
      "overlay-padding": 8,
    },
  },
  {
    selector: "node:selected",
    css: { "border-color": "#1c252b", "border-width": 4 },
  },
];

export function TopologyGraph(props: Props) {
  const container = useRef<HTMLDivElement>(null);
  const graph = useRef<Core | null>(null);
  const initialized = useRef(false);
  const layoutRuns = useRef(0);
  const renderEpoch = useRef(0);
  const topologyNodeIDs = useRef<string[]>([]);
  const cachedPositions = useRef(
    readLayoutCache(
      typeof window === "undefined" ? undefined : window.localStorage,
    ),
  );
  const elements = useMemo(
    () =>
      buildElements(props.topology, {
        pathFilter: props.pathFilter,
        showRecent: props.showRecent,
        query: props.query,
      }),
    [props.topology, props.pathFilter, props.showRecent, props.query],
  );

  useEffect(() => {
    if (!container.current) return;
    initialized.current = false;
    layoutRuns.current = 0;
    renderEpoch.current = 0;
    const cy = cytoscape({
      container: container.current,
      elements: [],
      style: styles,
      minZoom: 0.35,
      maxZoom: 2.2,
      boxSelectionEnabled: false,
    });
    graph.current = cy;
    cy.on("tap", "edge", (event) => {
      props.onSelectEdge(event.target.data("logicalEdgeId") as string);
      props.onSelectNode(null);
    });
    cy.on("tap", "node", (event) => {
      const logicalEdgeID = event.target.data("logicalEdgeId") as
        | string
        | undefined;
      if (logicalEdgeID) {
        props.onSelectEdge(logicalEdgeID);
        props.onSelectNode(null);
        return;
      }
      props.onSelectNode(event.target.id());
      props.onSelectEdge(null);
    });
    cy.on("tap", (event) => {
      if (event.target === cy) {
        props.onSelectEdge(null);
        props.onSelectNode(null);
      }
    });
    cy.on("free", "node[persistable]", () => persistPositionsNow(cy));
    cy.on("pan zoom", () =>
      updateGraphDiagnostics(cy, container.current, layoutRuns.current),
    );
    return () => {
      if (graph.current === cy) graph.current = null;
      cy.destroy();
    };
  }, []);

  useEffect(() => {
    const cy = graph.current;
    if (!cy) return;
    const epoch = ++renderEpoch.current;
    const firstRender = !initialized.current;
    const viewport = { zoom: cy.zoom(), pan: cy.pan() };
    captureCurrentPositions(cy, cachedPositions.current);
    topologyNodeIDs.current = props.topology.nodes.map((node) => node.id);
    const preparedElements = elements.map(withMeasuredIdealLength);
    const nextIDs = new Set(
      preparedElements.map((element) => String(element.data?.id)),
    );
    cy.elements().forEach((element) => {
      if (!nextIDs.has(element.id())) element.remove();
    });
    const newCanonicalNodes: NodeSingular[] = [];
    const knownNodeIDs = new Set<string>();
    for (const definition of preparedElements) {
      const id = String(definition.data?.id);
      const fingerprint = elementFingerprint(definition);
      nextFingerprints.set(id, fingerprint);
      const existing = cy.getElementById(id);
      if (existing.length) {
        existing.data(definition.data ?? {});
        existing.classes(definition.classes?.toString() ?? "");
        if (existing.isNode() && existing.data("persistable")) {
          knownNodeIDs.add(id);
        }
      } else {
        const added = cy.add(definition);
        if (added.isNode() && added.data("persistable")) {
          const cached = cachedPositions.current.get(id);
          if (cached) {
            added.position({ x: cached.x, y: cached.y });
            knownNodeIDs.add(id);
          } else {
            newCanonicalNodes.push(added);
          }
        }
      }
    }
    seedNewNodes(cy, newCanonicalNodes, knownNodeIDs);
    deriveVirtualPositions(cy);
    if (container.current) container.current.dataset.ready = "false";
    if (newCanonicalNodes.length > 0) {
      const newIDs = new Set(newCanonicalNodes.map((node) => node.id()));
      const locked: NodeSingular[] = [];
      cy.nodes().forEach((node) => {
        if (!newIDs.has(node.id())) {
          node.lock();
          locked.push(node);
        }
      });
      layoutRuns.current += 1;
      if (cy.nodes("[persistable]").length <= automaticCoseNodeLimit) {
        cy.layout({
          name: "cose",
          animate: false,
          randomize: firstRender && knownNodeIDs.size === 0,
          fit: false,
          padding: 64,
          nodeRepulsion: () => 180000,
          idealEdgeLength: (edge) => edge.data("idealLength") as number,
          edgeElasticity: () => 80,
          gravity: 45,
          componentSpacing: 120,
        }).run();
      }
      locked.forEach((node) => node.unlock());
      deriveVirtualPositions(cy);
    }
    initialized.current = true;
    if (!firstRender) {
      cy.zoom(viewport.zoom);
      cy.pan(viewport.pan);
    }
    requestAnimationFrame(() =>
      requestAnimationFrame(() => {
        if (
          graph.current !== cy ||
          cy.destroyed() ||
          epoch !== renderEpoch.current
        )
          return;
        cy.resize();
        if (firstRender && cy.nodes().length > 0) {
          cy.fit(cy.elements(), window.innerWidth <= 620 ? 28 : 72);
        }
        updateGraphDiagnostics(cy, container.current, layoutRuns.current);
        persistPositionsNow(cy);
      }),
    );
  }, [elements]);

  useEffect(() => {
    const cy = graph.current;
    if (!cy) return;
    cy.edges().unselect();
    if (props.selectedEdgeId) {
      cy.edges(
        `[logicalEdgeId = "${CSS.escape(props.selectedEdgeId)}"]`,
      ).select();
    }
  }, [props.selectedEdgeId]);

  function persistPositionsNow(cy: Core) {
    captureCurrentPositions(cy, cachedPositions.current);
    writeLayoutCache(
      typeof window === "undefined" ? undefined : window.localStorage,
      cachedPositions.current,
      topologyNodeIDs.current,
    );
    cachedPositions.current = readLayoutCache(
      typeof window === "undefined" ? undefined : window.localStorage,
    );
    updateGraphDiagnostics(cy, container.current, layoutRuns.current);
  }

  function fitGraph() {
    const cy = graph.current;
    if (!cy || cy.nodes().length === 0) return;
    cy.fit(cy.elements(), window.innerWidth <= 620 ? 28 : 72);
    updateGraphDiagnostics(cy, container.current, layoutRuns.current);
  }

  function relayoutGraph() {
    const cy = graph.current;
    if (!cy || cy.nodes().length === 0) return;
    clearLayoutCache(
      typeof window === "undefined" ? undefined : window.localStorage,
    );
    cachedPositions.current.clear();
    cy.nodes().unlock();
    layoutRuns.current += 1;
    cy.layout({
      name: "cose",
      animate: false,
      randomize: true,
      fit: false,
      padding: 64,
      nodeRepulsion: () => 180000,
      idealEdgeLength: (edge) => edge.data("idealLength") as number,
      edgeElasticity: () => 80,
      gravity: 45,
      componentSpacing: 120,
    }).run();
    deriveVirtualPositions(cy);
    cy.fit(cy.elements(), window.innerWidth <= 620 ? 28 : 72);
    persistPositionsNow(cy);
  }

  return (
    <>
      <div
        className="topology-canvas"
        ref={container}
        aria-label="Live Tailnet topology"
        data-edge-count={
          new Set(
            elements
              .filter((element) => element.group === "edges")
              .map((element) => element.data?.logicalEdgeId),
          ).size
        }
        data-node-count={
          elements.filter((element) => element.group === "nodes").length
        }
      />
      <div className="graph-controls" aria-label="Graph layout controls">
        <button
          type="button"
          onClick={fitGraph}
          title="Fit graph"
          aria-label="Fit graph"
        >
          <Maximize2 size={16} />
        </button>
        <button
          type="button"
          onClick={relayoutGraph}
          title="Relayout graph"
          aria-label="Relayout graph"
        >
          <RefreshCcw size={16} />
        </button>
      </div>
    </>
  );
}

const measuredLabels = new Map<string, number>();

function withMeasuredIdealLength(
  definition: ElementDefinition,
): ElementDefinition {
  if (definition.group !== "edges" || !definition.data?.label)
    return definition;
  const label = String(definition.data.label);
  let labelWidth = measuredLabels.get(label);
  if (labelWidth === undefined) {
    const context = document.createElement("canvas").getContext("2d");
    if (context) {
      context.font = "600 8px Inter, ui-sans-serif, system-ui, sans-serif";
      labelWidth = Math.ceil(context.measureText(label).width);
      measuredLabels.set(label, labelWidth);
    }
  }
  if (labelWidth === undefined) return definition;
  return {
    ...definition,
    data: {
      ...definition.data,
      idealLength: edgeIdealLengthForWidth(labelWidth),
    },
  };
}

function captureCurrentPositions(cy: Core, cache: Map<string, LayoutPosition>) {
  const now = Date.now();
  cy.nodes("[persistable]").forEach((node) => {
    const position = node.position();
    if (Number.isFinite(position.x) && Number.isFinite(position.y)) {
      cache.set(node.id(), { x: position.x, y: position.y, lastSeen: now });
    }
  });
}

function seedNewNodes(
  cy: Core,
  nodes: NodeSingular[],
  knownNodeIDs: ReadonlySet<string>,
) {
  nodes.forEach((node, index) => {
    const neighbors = knownNeighborPositions(node, knownNodeIDs);
    if (neighbors.length > 0) {
      const center = neighbors.reduce(
        (sum, position) => ({
          x: sum.x + position.x / neighbors.length,
          y: sum.y + position.y / neighbors.length,
        }),
        { x: 0, y: 0 },
      );
      const angle = (stableHash(node.id()) % 360) * (Math.PI / 180);
      node.position({
        x: center.x + Math.cos(angle) * 72,
        y: center.y + Math.sin(angle) * 72,
      });
      return;
    }
    const columns = Math.max(1, Math.ceil(Math.sqrt(nodes.length)));
    const offset = stableHash(node.id()) % 19;
    node.position({
      x: (index % columns) * 140 + offset,
      y: Math.floor(index / columns) * 140 + offset,
    });
  });
  deriveVirtualPositions(cy);
}

function knownNeighborPositions(
  node: NodeSingular,
  knownNodeIDs: ReadonlySet<string>,
) {
  const positions = new Map<string, { x: number; y: number }>();
  const visit = (candidate: NodeSingular) => {
    if (candidate.id() !== node.id() && knownNodeIDs.has(candidate.id())) {
      positions.set(candidate.id(), candidate.position());
    }
  };
  node.neighborhood("node").forEach((neighbor) => {
    if (!neighbor.isNode()) return;
    visit(neighbor);
    if (!neighbor.data("persistable")) {
      neighbor.neighborhood("node").forEach((candidate) => {
        if (candidate.isNode()) visit(candidate);
      });
    }
  });
  return [...positions.values()];
}

function deriveVirtualPositions(cy: Core) {
  cy.nodes().forEach((node) => {
    if (node.data("persistable")) return;
    const neighbors: Array<{ x: number; y: number }> = [];
    node.neighborhood("node").forEach((neighbor) => {
      if (neighbor.isNode()) neighbors.push(neighbor.position());
    });
    if (neighbors.length === 0) return;
    node.position(
      neighbors.reduce(
        (sum, position) => ({
          x: sum.x + position.x / neighbors.length,
          y: sum.y + position.y / neighbors.length,
        }),
        { x: 0, y: 0 },
      ),
    );
  });
}

function updateGraphDiagnostics(
  cy: Core,
  element: HTMLDivElement | null,
  runs: number,
) {
  if (!element) return;
  const deviceNodes = cy.nodes(".device-node");
  let deviceNodesSquare = true;
  deviceNodes.forEach((node) => {
    if (!node.isNode() || node.width() !== 52 || node.height() !== 52) {
      deviceNodesSquare = false;
    }
  });
  const positions: string[] = [];
  cy.nodes("[persistable]").forEach((node) => {
    const position = node.position();
    positions.push(
      `${node.id()}:${position.x.toFixed(2)},${position.y.toFixed(2)}`,
    );
  });
  positions.sort();
  const pan = cy.pan();
  element.dataset.deviceNodeCount = String(deviceNodes.length);
  element.dataset.deviceNodesSquare = String(deviceNodesSquare);
  element.dataset.layoutPositions = positions.join("|");
  element.dataset.layoutRuns = String(runs);
  element.dataset.viewport = `${cy.zoom().toFixed(4)}:${pan.x.toFixed(2)},${pan.y.toFixed(2)}`;
  element.dataset.ready = "true";
}

function stableHash(value: string): number {
  let result = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    result ^= value.charCodeAt(index);
    result = Math.imul(result, 16777619);
  }
  return result >>> 0;
}
