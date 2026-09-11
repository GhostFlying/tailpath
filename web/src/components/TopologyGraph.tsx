import { quadratic } from "../lib/layoutGeometry";
import { useEffect, useMemo, useRef, useState } from "react";
import { Maximize2, RefreshCcw } from "lucide-react";
import cytoscape, {
  type Core,
  type CollectionReturnValue,
  type ElementDefinition,
  type NodeSingular,
  type StylesheetCSS,
} from "cytoscape";
import {
  createLayoutPresentation,
  clearFontMeasurements,
  type PresentationSummary,
} from "../lib/layoutPresentation";
import type { Topology } from "../api/types";
import {
  buildElements,
  edgeIdealLengthForWidth,
  minimumEdgeCenterDistance,
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
  selectedNodeId: string | null;
  focusNodeId: string | null;
  onSelectEdge: (edgeId: string | null) => void;
  onSelectNode: (nodeId: string | null) => void;
}

const automaticCoseNodeLimit = 100;
const sparseGraphNodeLimit = 12;
const maximumSparseZoom = 1.25;
const obstaclePadding = 14;
const virtualCandidateStep = 48;
const maximumVirtualCandidateSteps = 6;
const maximumObstacleRoutingNodes = 64;
const maximumObstacleRoutingEdges = 128;

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
    selector: "node.identity-partial, node.identity-anonymous",
    css: {
      "border-style": "dashed",
      "border-color": "#8a6500",
      "background-color": "#fffdf5",
    },
  },
  {
    selector: "node.identity-conflict",
    css: {
      "border-style": "double",
      "border-width": 4,
      "border-color": "#b64141",
      "background-color": "#fff5f5",
    },
  },
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
  const fitPasses = useRef(0);
  const fitting = useRef(false);
  const presentation = useRef<ReturnType<
    typeof createLayoutPresentation
  > | null>(null);
  const [summary, setSummary] = useState<PresentationSummary>({
    mode: "detail",
    hidden: 0,
    collisions: 0,
  });
  const callbacks = useRef(props);
  callbacks.current = props;
  const initialized = useRef(false);
  const layoutRuns = useRef(0);
  const renderEpoch = useRef(0);
  const focusPending = useRef(true);
  const renderedTopologyAt = useRef<string | null>(null);
  const renderedVisibility = useRef<string | null>(null);
  const renderedGeometry = useRef<string | null>(null);
  const topologyNodeIDs = useRef<string[]>([]);
  const renderedFingerprints = useRef(new Map<string, string>());
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
    focusPending.current = true;
    renderedTopologyAt.current = null;
    renderedVisibility.current = null;
    renderedGeometry.current = null;
    renderedFingerprints.current.clear();
    const cy = cytoscape({
      container: container.current,
      elements: [],
      style: styles,
      minZoom: 0.1,
      maxZoom: 2.2,
      boxSelectionEnabled: false,
    });
    graph.current = cy;
    presentation.current = createLayoutPresentation(cy, container.current, {
      reroute: (moveVirtual) => {
        if (moveVirtual && !fitting.current) deriveVirtualPositions(cy);
        routeEdgesAroundObstacles(cy);
      },
      onComplete: (next) => {
        if (fitPasses.current > 0) {
          fitting.current = true;
          fitPasses.current--;
          focusGraph(cy, graphPadding());
          presentation.current?.request();
          return;
        }
        fitting.current = false;
        if (container.current) container.current.dataset.ready = "true";
        updateGraphDiagnostics(cy, container.current, layoutRuns.current);
        setSummary((previous) =>
          previous.mode === next.mode &&
          previous.hidden === next.hidden &&
          previous.collisions === next.collisions
            ? previous
            : next,
        );
      },
    });
    const refreshPresentation = () => presentation.current?.request();
    cy.on("zoom resize free select unselect", refreshPresentation);
    const resize = new ResizeObserver(() => {
      cy.resize();
      refreshPresentation();
    });
    resize.observe(container.current);
    const inspector = document.querySelector(".inspector");
    if (inspector) resize.observe(inspector);
    const fontsChanged = () => {
      clearFontMeasurements();
      refreshPresentation();
    };
    document.fonts.addEventListener("loadingdone", fontsChanged);
    void document.fonts.ready.then(() => {
      if (!cy.destroyed()) fontsChanged();
    });
    cy.on("tap", "edge", (event) => {
      callbacks.current.onSelectEdge(
        event.target.data("logicalEdgeId") as string,
      );
      callbacks.current.onSelectNode(null);
    });
    cy.on("tap", "node", (event) => {
      const logicalEdgeID = event.target.data("logicalEdgeId") as
        | string
        | undefined;
      if (logicalEdgeID) {
        callbacks.current.onSelectEdge(logicalEdgeID);
        callbacks.current.onSelectNode(null);
        return;
      }
      callbacks.current.onSelectNode(event.target.id());
      callbacks.current.onSelectEdge(null);
    });
    cy.on("tap", (event) => {
      if (event.target === cy) {
        callbacks.current.onSelectEdge(null);
        callbacks.current.onSelectNode(null);
      }
    });
    cy.on("free", "node[persistable]", () => {
      deriveVirtualPositions(cy);
      routeEdgesAroundObstacles(cy);
      persistPositionsNow(cy);
    });
    cy.on("pan zoom", () =>
      updateGraphDiagnostics(cy, container.current, layoutRuns.current),
    );
    return () => {
      resize.disconnect();
      document.fonts.removeEventListener("loadingdone", fontsChanged);
      presentation.current?.dispose();
      presentation.current = null;
      if (graph.current === cy) graph.current = null;
      cy.destroy();
    };
  }, []);

  useEffect(() => {
    const cy = graph.current;
    if (!cy) return;
    const epoch = ++renderEpoch.current;
    const firstRender = !initialized.current;
    const topologyChanged =
      renderedTopologyAt.current !== props.topology.generatedAt;
    renderedTopologyAt.current = props.topology.generatedAt;
    const visibility = `${props.pathFilter}:${props.showRecent}`;
    const visibilityChanged = renderedVisibility.current !== visibility;
    renderedVisibility.current = visibility;
    const viewport = { zoom: cy.zoom(), pan: cy.pan() };
    captureCurrentPositions(cy, cachedPositions.current);
    topologyNodeIDs.current = props.topology.nodes.map((node) => node.id);
    const preparedElements = elements.map(withMeasuredIdealLength);
    const geometry = geometryFingerprint(preparedElements);
    const geometryChanged = renderedGeometry.current !== geometry;
    renderedGeometry.current = geometry;
    const previousCanonicalIDs = new Set(
      cy.nodes("[persistable]").map((node) => node.id()),
    );
    const nextCanonicalIDs = new Set(
      preparedElements
        .filter(
          (element) =>
            element.group === "nodes" && Boolean(element.data?.persistable),
        )
        .map((element) => String(element.data?.id)),
    );
    const structureChanged = !sameIDs(previousCanonicalIDs, nextCanonicalIDs);
    const hasSharedCanonicalNode = [...nextCanonicalIDs].some((id) =>
      previousCanonicalIDs.has(id),
    );
    if (visibilityChanged && !firstRender) {
      focusPending.current = false;
    } else if (nextCanonicalIDs.size === 0 && topologyChanged) {
      focusPending.current = true;
    } else if (firstRender || (topologyChanged && !hasSharedCanonicalNode)) {
      focusPending.current = true;
    }
    const shouldFocusTopology =
      nextCanonicalIDs.size > 0 && focusPending.current;
    const nextIDs = new Set(
      preparedElements.map((element) => String(element.data?.id)),
    );
    cy.elements().forEach((element) => {
      if (!nextIDs.has(element.id())) element.remove();
    });
    const newCanonicalNodes: NodeSingular[] = [];
    const knownNodeIDs = new Set<string>();
    const nextFingerprints = new Map<string, string>();
    for (const definition of preparedElements) {
      const id = String(definition.data?.id);
      const fingerprint = elementFingerprint(definition);
      nextFingerprints.set(id, fingerprint);
      const existing = cy.getElementById(id);
      if (existing.length) {
        if (renderedFingerprints.current.get(id) !== fingerprint) {
          updateElementIfChanged(existing, definition);
        }
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
    renderedFingerprints.current = nextFingerprints;
    seedNewNodes(cy, newCanonicalNodes, knownNodeIDs);
    deriveVirtualPositions(cy);
    if (container.current) container.current.dataset.ready = "false";
    const movableNodeIDs = new Set(newCanonicalNodes.map((node) => node.id()));
    if (newCanonicalNodes.length > 0) {
      const locked: NodeSingular[] = [];
      cy.nodes().forEach((node) => {
        if (!movableNodeIDs.has(node.id())) {
          node.lock();
          locked.push(node);
        }
      });
      layoutRuns.current += 1;
      if (cy.nodes("[persistable]").length <= automaticCoseNodeLimit) {
        cy.layout({
          name: "cose",
          animate: false,
          randomize: false,
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
    }
    if (structureChanged) {
      enforceSparseEdgeClearance(cy, movableNodeIDs);
      enforceFootprintClearance(cy, movableNodeIDs);
    }
    deriveVirtualPositions(cy);
    if (geometryChanged || newCanonicalNodes.length > 0) {
      routeEdgesAroundObstacles(cy);
    }
    initialized.current = true;
    if (!firstRender && !shouldFocusTopology) {
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
        if (geometryChanged || newCanonicalNodes.length > 0) {
          deriveVirtualPositions(cy);
          routeEdgesAroundObstacles(cy);
        }
        if (shouldFocusTopology) {
          fitPasses.current = 3;
          focusGraph(cy, graphPadding());
          focusPending.current = false;
        }
        updateGraphDiagnostics(cy, container.current, layoutRuns.current);
        persistPositionsNow(cy);
        presentation.current?.request();
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

  useEffect(() => {
    const cy = graph.current;
    if (!cy) return;
    cy.nodes().unselect();
    if (props.selectedNodeId) {
      cy.getElementById(props.selectedNodeId).select();
    }
  }, [props.selectedNodeId]);

  useEffect(() => {
    const cy = graph.current;
    if (!cy || !props.focusNodeId) return;
    const node = cy.getElementById(props.focusNodeId);
    if (!node.length) return;
    requestAnimationFrame(() =>
      requestAnimationFrame(() => {
        if (cy.destroyed() || !node.inside()) return;
        cy.animate({
          center: { eles: node },
          zoom: Math.max(cy.zoom(), 1),
          duration: 180,
        });
      }),
    );
  }, [props.focusNodeId]);

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
    fitPasses.current = 3;
    const cy = graph.current;
    if (!cy || cy.nodes().length === 0) return;
    focusGraph(cy, graphPadding());
    presentation.current?.request();
    updateGraphDiagnostics(cy, container.current, layoutRuns.current);
  }

  function relayoutGraph() {
    fitPasses.current = 3;
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
    enforceSparseEdgeClearance(cy);
    enforceFootprintClearance(cy);
    deriveVirtualPositions(cy);
    routeEdgesAroundObstacles(cy);
    focusGraph(cy, graphPadding());
    persistPositionsNow(cy);
    presentation.current?.request();
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
        data-selected-node-id={props.selectedNodeId ?? ""}
      />
      <div className="graph-detail-status" role="status">
        {summary.hidden > 0
          ? `${summary.hidden} labels hidden · Zoom or select to inspect`
          : ""}
        {summary.collisions > 0
          ? " · Some saved positions are crowded. Use Relayout to spread nodes."
          : ""}
      </div>
      <details
        className="graph-objects"
        onToggle={() => presentation.current?.request()}
      >
        <summary>Graph objects</summary>
        <div className="graph-object-list" aria-label="Visible graph objects">
          {elements
            .filter((e) => e.group === "nodes")
            .map((e) => (
              <button
                key={String(e.data?.id)}
                type="button"
                onClick={() => {
                  if (e.data?.logicalEdgeId) {
                    callbacks.current.onSelectEdge(
                      String(e.data.logicalEdgeId),
                    );
                    callbacks.current.onSelectNode(null);
                  } else {
                    callbacks.current.onSelectNode(String(e.data?.id));
                    callbacks.current.onSelectEdge(null);
                  }
                }}
              >
                {String(e.data?.label)}
              </button>
            ))}
          {[
            ...new Map(
              elements
                .filter((e) => e.group === "edges")
                .map((e) => [String(e.data?.logicalEdgeId), e]),
            ).entries(),
          ].map(([id, e]) => (
            <button
              key={id}
              type="button"
              onClick={() => {
                callbacks.current.onSelectEdge(id);
                callbacks.current.onSelectNode(null);
              }}
            >
              {props.topology.edges.find((edge) => edge.id === id)?.source} →{" "}
              {props.topology.edges.find((edge) => edge.id === id)?.target} ·{" "}
              {String(e.data?.label || "Recent")}
            </button>
          ))}
        </div>
      </details>
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

function updateElementIfChanged(
  element: CollectionReturnValue,
  definition: ElementDefinition,
) {
  const nextData = definition.data ?? {};
  const currentData = element.data() as Record<string, unknown>;
  const dataChanged =
    Object.keys(currentData).length !== Object.keys(nextData).length ||
    Object.entries(nextData).some(
      ([key, value]) => !sameElementData(currentData[key], value),
    );
  if (dataChanged) element.data(nextData);

  const nextClasses = String(definition.classes ?? "")
    .split(/\s+/)
    .filter(Boolean);
  const currentClasses = element.classes();
  if (
    currentClasses.length !== nextClasses.length ||
    nextClasses.some((className) => !element.hasClass(className))
  ) {
    element.classes(nextClasses);
  }
}

function elementFingerprint(definition: ElementDefinition): string {
  return JSON.stringify([definition.data ?? {}, definition.classes ?? ""]);
}

function geometryFingerprint(definitions: ElementDefinition[]): string {
  return definitions
    .map((definition) => {
      const data = definition.data ?? {};
      return definition.group === "edges"
        ? `e:${String(data.id)}:${String(data.source)}:${String(data.target)}`
        : `n:${String(data.id)}:${String(data.label)}:${String(data.kind)}`;
    })
    .sort()
    .join("|");
}

function sameElementData(left: unknown, right: unknown): boolean {
  if (left === right) return true;
  if (!Array.isArray(left) || !Array.isArray(right)) return false;
  return (
    left.length === right.length &&
    left.every((value, index) => value === right[index])
  );
}

function sameIDs(left: ReadonlySet<string>, right: ReadonlySet<string>) {
  if (left.size !== right.size) return false;
  for (const id of left) {
    if (!right.has(id)) return false;
  }
  return true;
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
  const extent = cy.extent();
  const viewportCenter = {
    x: (extent.x1 + extent.x2) / 2,
    y: (extent.y1 + extent.y2) / 2,
  };
  const columns = Math.max(1, Math.ceil(Math.sqrt(nodes.length)));
  const rows = Math.max(1, Math.ceil(nodes.length / columns));
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
    const offset = (stableHash(node.id()) % 19) - 9;
    node.position({
      x:
        viewportCenter.x +
        ((index % columns) - (columns - 1) / 2) * minimumEdgeCenterDistance +
        offset,
      y:
        viewportCenter.y +
        (Math.floor(index / columns) - (rows - 1) / 2) *
          minimumEdgeCenterDistance +
        offset,
    });
  });
  deriveVirtualPositions(cy);
}

function enforceSparseEdgeClearance(
  cy: Core,
  movableNodeIDs?: ReadonlySet<string>,
) {
  if (cy.nodes("[persistable]").length > sparseGraphNodeLimit) return;
  for (let pass = 0; pass < sparseGraphNodeLimit; pass += 1) {
    let adjusted = false;
    cy.edges().forEach((edge) => {
      const source = edge.source();
      const target = edge.target();
      if (
        !source.data("persistable") ||
        !target.data("persistable") ||
        source.id() === target.id()
      ) {
        return;
      }
      const sourceMovable = movableNodeIDs?.has(source.id()) ?? true;
      const targetMovable = movableNodeIDs?.has(target.id()) ?? true;
      if (!sourceMovable && !targetMovable) return;
      const desired = Math.max(
        minimumEdgeCenterDistance,
        Number(edge.data("idealLength")) || 0,
      );
      const sourcePosition = source.position();
      const targetPosition = target.position();
      let dx = targetPosition.x - sourcePosition.x;
      let dy = targetPosition.y - sourcePosition.y;
      let distance = Math.hypot(dx, dy);
      if (distance + 0.5 >= desired) return;
      if (distance < 0.001) {
        const angle = (stableHash(edge.id()) % 360) * (Math.PI / 180);
        dx = Math.cos(angle);
        dy = Math.sin(angle);
        distance = 1;
      }
      const shift =
        (desired - distance) / (sourceMovable && targetMovable ? 2 : 1);
      const unitX = dx / distance;
      const unitY = dy / distance;
      if (sourceMovable) {
        source.position({
          x: sourcePosition.x - unitX * shift,
          y: sourcePosition.y - unitY * shift,
        });
      }
      if (targetMovable) {
        target.position({
          x: targetPosition.x + unitX * shift,
          y: targetPosition.y + unitY * shift,
        });
      }
      adjusted = true;
    });
    if (!adjusted) break;
  }
}

function enforceFootprintClearance(cy: Core, movable?: ReadonlySet<string>) {
  if (cy.nodes("[persistable]").length > automaticCoseNodeLimit) return;
  const nodes = cy
    .nodes("[persistable]")
    .sort((a, b) => a.id().localeCompare(b.id()));
  for (let pass = 0; pass < 12; pass++) {
    let changed = false;
    for (let i = 0; i < nodes.length; i++)
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i],
          b = nodes[j];
        const am = !movable || movable.has(a.id()),
          bm = !movable || movable.has(b.id());
        if (!am && !bm) continue;
        const ab = paddedNodeBounds(a, 6),
          bb = paddedNodeBounds(b, 6);
        if (!boundsOverlap(ab, bb)) continue;
        const dx = Math.min(ab.x2 - bb.x1, bb.x2 - ab.x1) + 1;
        const dy = Math.min(ab.y2 - bb.y1, bb.y2 - ab.y1) + 1;
        const axis = dx <= dy ? "x" : "y";
        const sign = a.position(axis) <= b.position(axis) ? -1 : 1;
        const shift = (axis === "x" ? dx : dy) / (am && bm ? 2 : 1);
        if (am) a.position(axis, a.position(axis) + sign * shift);
        if (bm) b.position(axis, b.position(axis) - sign * shift);
        changed = true;
      }
    if (!changed) break;
  }
}

function focusGraph(cy: Core, padding: number) {
  const bounds = cy
    .elements()
    .boundingBox({ includeLabels: true, includeOverlays: false });
  if (bounds.w <= 0 || bounds.h <= 0) return;
  // Reserve controls above and the readable detail status below the topology.
  const top = 72,
    bottom = 40;
  let right = padding;
  const containerBounds = cy.container()?.getBoundingClientRect();
  const inspector = document
    .querySelector(".inspector")
    ?.getBoundingClientRect();
  let availableBottom = cy.height() - bottom;
  if (
    containerBounds &&
    inspector &&
    inspector.left < containerBounds.right &&
    inspector.bottom > containerBounds.top
  ) {
    if (window.innerWidth <= 620)
      availableBottom = Math.min(
        availableBottom,
        inspector.top - containerBounds.top - padding,
      );
    else
      right = Math.max(right, containerBounds.right - inspector.left + padding);
  }
  const width = Math.max(1, cy.width() - padding - right);
  const height = Math.max(1, availableBottom - top);
  const zoom = Math.max(
    cy.minZoom(),
    Math.min(
      cy.maxZoom(),
      maximumSparseZoom,
      width / bounds.w,
      height / bounds.h,
    ),
  );
  cy.zoom(zoom);
  cy.pan({
    x: padding + width / 2 - ((bounds.x1 + bounds.x2) / 2) * zoom,
    y: top + height / 2 - ((bounds.y1 + bounds.y2) / 2) * zoom,
  });
}

function graphPadding() {
  return 16;
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
  const occupied = cy
    .nodes("[persistable]")
    .map((node) => paddedNodeBounds(node, obstaclePadding));
  const canonicalEdges: Array<{
    source: Point & { id: string };
    target: Point & { id: string };
  }> = [];
  if (cy.edges().length <= maximumObstacleRoutingEdges) {
    cy.edges().forEach((edge) => {
      if (
        edge.source().data("persistable") &&
        edge.target().data("persistable")
      ) {
        canonicalEdges.push({
          source: { id: edge.source().id(), ...edge.source().position() },
          target: { id: edge.target().id(), ...edge.target().position() },
        });
      }
    });
  }
  const virtualNodes = cy
    .nodes()
    .filter((node) => !node.data("persistable"))
    .sort((left, right) => left.id().localeCompare(right.id()));
  virtualNodes.forEach((node) => {
    const neighbors: Array<Point & { id: string }> = [];
    node.neighborhood("node").forEach((neighbor) => {
      if (neighbor.isNode()) {
        neighbors.push({ id: neighbor.id(), ...neighbor.position() });
      }
    });
    neighbors.sort((left, right) => left.id.localeCompare(right.id));
    if (neighbors.length === 0) return;
    const origin = neighbors.reduce(
      (sum, position) => ({
        x: sum.x + position.x / neighbors.length,
        y: sum.y + position.y / neighbors.length,
      }),
      { x: 0, y: 0 },
    );
    const axis = virtualOffsetAxis(neighbors, node.id());
    const tangent = { x: axis.y, y: -axis.x };
    const preferredSide = stableHash(node.id()) % 2 === 0 ? 1 : -1;
    const candidates = [node.position(), origin];
    for (let step = 1; step <= maximumVirtualCandidateSteps; step += 1) {
      for (const side of [preferredSide, -preferredSide]) {
        for (const lean of [0, 0.65, -0.65]) {
          const direction = normalizePoint({
            x: axis.x * side + tangent.x * lean,
            y: axis.y * side + tangent.y * lean,
          });
          candidates.push({
            x: origin.x + direction.x * virtualCandidateStep * step,
            y: origin.y + direction.y * virtualCandidateStep * step,
          });
        }
      }
    }
    for (const candidate of candidates) {
      node.position(candidate);
      const bounds = paddedNodeBounds(node, obstaclePadding);
      if (
        !occupied.some((obstacle) => boundsOverlap(bounds, obstacle)) &&
        !canonicalEdges.some((edge) =>
          segmentIntersectsBounds(edge.source, edge.target, bounds),
        ) &&
        !neighbors.some((neighbor) =>
          canonicalEdges.some(
            (edge) =>
              edge.source.id !== neighbor.id &&
              edge.target.id !== neighbor.id &&
              segmentsCross(neighbor, candidate, edge.source, edge.target),
          ),
        )
      ) {
        occupied.push(bounds);
        return;
      }
    }
    occupied.push(paddedNodeBounds(node, obstaclePadding));
  });
}

function normalizePoint(point: Point) {
  const length = Math.hypot(point.x, point.y);
  return length > 0.001
    ? { x: point.x / length, y: point.y / length }
    : { x: 1, y: 0 };
}

interface Point {
  x: number;
  y: number;
}

interface Bounds {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

interface RoutedPath {
  logicalEdgeID: string;
  nodeIDs: ReadonlySet<string>;
  points: Point[];
}

function virtualOffsetAxis(
  neighbors: Array<Point & { id: string }>,
  id: string,
) {
  if (neighbors.length >= 2) {
    const dx = neighbors[neighbors.length - 1].x - neighbors[0].x;
    const dy = neighbors[neighbors.length - 1].y - neighbors[0].y;
    const length = Math.hypot(dx, dy);
    if (length > 0.001) return { x: -dy / length, y: dx / length };
  }
  const angle = (stableHash(id) % 360) * (Math.PI / 180);
  return { x: Math.cos(angle), y: Math.sin(angle) };
}

function paddedNodeBounds(node: NodeSingular, padding: number): Bounds {
  padding /= node.cy().zoom();
  const bounds = node.boundingBox({
    includeLabels: true,
    includeOverlays: false,
  });
  return {
    x1: bounds.x1 - padding,
    y1: bounds.y1 - padding,
    x2: bounds.x2 + padding,
    y2: bounds.y2 + padding,
  };
}

function boundsOverlap(left: Bounds, right: Bounds) {
  return !(
    left.x2 < right.x1 ||
    left.x1 > right.x2 ||
    left.y2 < right.y1 ||
    left.y1 > right.y2
  );
}

function routeEdgesAroundObstacles(cy: Core) {
  cy.edges().removeStyle(
    "curve-style control-point-distances control-point-weights",
  );
  cy.edges().forEach((edge) => {
    edge.scratch("tailpathObstacleRouted", false);
    edge.removeScratch("tailpathRoute");
  });
  // Dense graphs favor bounded render cost; the default filtered Live view is
  // where obstacle routing materially improves readability.
  if (
    cy.nodes().length > maximumObstacleRoutingNodes ||
    cy.edges().length > maximumObstacleRoutingEdges
  ) {
    return;
  }
  const nodes = cy.nodes().map((node) => ({
    id: node.id(),
    bounds: paddedNodeBounds(node, obstaclePadding),
  }));
  const routedPaths: RoutedPath[] = [];
  const edges = cy.edges().sort((left, right) => {
    const leftVirtual =
      Number(!left.source().data("persistable")) +
      Number(!left.target().data("persistable"));
    const rightVirtual =
      Number(!right.source().data("persistable")) +
      Number(!right.target().data("persistable"));
    return leftVirtual - rightVirtual || left.id().localeCompare(right.id());
  });
  edges.forEach((edge) => {
    const source = edge.source().position();
    const target = edge.target().position();
    const nodeIDs = new Set([edge.source().id(), edge.target().id()]);
    const logicalEdgeID = String(edge.data("logicalEdgeId"));
    const unrelatedPaths = routedPaths.filter(
      (path) =>
        path.logicalEdgeID !== logicalEdgeID &&
        ![...nodeIDs].some((id) => path.nodeIDs.has(id)),
    );
    const obstacles = nodes
      .filter(
        (node) =>
          node.id !== edge.source().id() && node.id !== edge.target().id(),
      )
      .map((node) => node.bounds);
    const straightPoints = [source, target];
    const intersectsObstacle = obstacles.some((bounds) =>
      segmentIntersectsBounds(source, target, bounds),
    );
    const intersectsPath = unrelatedPaths.some((path) =>
      pathsCross(straightPoints, path.points),
    );
    if (!intersectsObstacle && !intersectsPath) {
      const path = { logicalEdgeID, nodeIDs, points: straightPoints };
      routedPaths.push(path);
      edge.scratch("tailpathRoute", path);
      return;
    }
    const route = findClearCurve(
      source,
      target,
      obstacles,
      unrelatedPaths,
      edge.id(),
    );
    if (!route) {
      const path = { logicalEdgeID, nodeIDs, points: straightPoints };
      routedPaths.push(path);
      edge.scratch("tailpathRoute", path);
      return;
    }
    edge.style({
      "curve-style": "unbundled-bezier",
      "control-point-weights": route.weight,
      "control-point-distances": route.distance,
    });
    edge.scratch("tailpathObstacleRouted", true);
    const path = { logicalEdgeID, nodeIDs, points: route.points };
    routedPaths.push(path);
    edge.scratch("tailpathRoute", path);
  });
}

function findClearCurve(
  source: Point,
  target: Point,
  obstacles: Bounds[],
  occupiedPaths: RoutedPath[],
  id: string,
) {
  const dx = target.x - source.x;
  const dy = target.y - source.y;
  const length = Math.hypot(dx, dy);
  if (length < 0.001) return null;
  const collisionWeights = obstacles
    .filter((bounds) => segmentIntersectsBounds(source, target, bounds))
    .map((bounds) => {
      const center = {
        x: (bounds.x1 + bounds.x2) / 2,
        y: (bounds.y1 + bounds.y2) / 2,
      };
      return Math.max(
        0.15,
        Math.min(
          0.85,
          ((center.x - source.x) * dx + (center.y - source.y) * dy) /
            (length * length),
        ),
      );
    });
  const weights = [...new Set([...collisionWeights, 0.5])].sort(
    (left, right) => Math.abs(left - 0.5) - Math.abs(right - 0.5),
  );
  const preferredSide = stableHash(id) % 2 === 0 ? 1 : -1;
  const maximumDistance = Math.max(384, length * 0.8);
  for (let magnitude = 64; magnitude <= maximumDistance; magnitude += 32) {
    for (const side of [preferredSide, -preferredSide]) {
      for (const weight of weights) {
        const distance = magnitude * side;
        const points = curvePoints(source, target, weight, distance);
        if (
          !pointsIntersectBounds(points, obstacles) &&
          !occupiedPaths.some((path) => pathsCross(points, path.points))
        ) {
          return { weight, distance, points };
        }
      }
    }
  }
  return null;
}

function curvePoints(
  source: Point,
  target: Point,
  weight: number,
  distance: number,
) {
  const dx = target.x - source.x;
  const dy = target.y - source.y;
  const length = Math.hypot(dx, dy);
  const control = {
    x: source.x + dx * weight - (dy / length) * distance,
    y: source.y + dy * weight + (dx / length) * distance,
  };
  return quadratic(source, control, target, 0.25);
}

function pointsIntersectBounds(points: Point[], obstacles: Bounds[]) {
  for (let index = 1; index < points.length; index += 1) {
    if (
      obstacles.some((bounds) =>
        segmentIntersectsBounds(points[index - 1], points[index], bounds),
      )
    ) {
      return true;
    }
  }
  return false;
}

function pathsCross(left: Point[], right: Point[]) {
  for (let leftIndex = 1; leftIndex < left.length; leftIndex += 1) {
    for (let rightIndex = 1; rightIndex < right.length; rightIndex += 1) {
      if (
        segmentsCross(
          left[leftIndex - 1],
          left[leftIndex],
          right[rightIndex - 1],
          right[rightIndex],
        )
      ) {
        return true;
      }
    }
  }
  return false;
}

function segmentsCross(a: Point, b: Point, c: Point, d: Point) {
  const abC = crossProduct(a, b, c);
  const abD = crossProduct(a, b, d);
  const cdA = crossProduct(c, d, a);
  const cdB = crossProduct(c, d, b);
  const epsilon = 1e-6;
  return (
    ((abC > epsilon && abD < -epsilon) || (abC < -epsilon && abD > epsilon)) &&
    ((cdA > epsilon && cdB < -epsilon) || (cdA < -epsilon && cdB > epsilon))
  );
}

function crossProduct(a: Point, b: Point, point: Point) {
  return (b.x - a.x) * (point.y - a.y) - (b.y - a.y) * (point.x - a.x);
}

function segmentIntersectsBounds(start: Point, end: Point, bounds: Bounds) {
  let minimum = 0;
  let maximum = 1;
  const dx = end.x - start.x;
  const dy = end.y - start.y;
  for (const [origin, delta, lower, upper] of [
    [start.x, dx, bounds.x1, bounds.x2],
    [start.y, dy, bounds.y1, bounds.y2],
  ]) {
    if (Math.abs(delta) < 1e-9) {
      if (origin < lower || origin > upper) return false;
      continue;
    }
    const first = (lower - origin) / delta;
    const second = (upper - origin) / delta;
    minimum = Math.max(minimum, Math.min(first, second));
    maximum = Math.min(maximum, Math.max(first, second));
    if (minimum > maximum) return false;
  }
  return true;
}

function updateGraphDiagnostics(
  cy: Core,
  element: HTMLDivElement | null,
  runs: number,
) {
  if (!element) return;
  const deviceNodes = cy.nodes(".device-node");
  const relayNodes = cy.nodes(".peer-relay");
  let deviceNodesSquare = true;
  let relayPlatformIconCount = 0;
  deviceNodes.forEach((node) => {
    if (!node.isNode() || node.width() !== 52 || node.height() !== 52) {
      deviceNodesSquare = false;
    }
  });
  relayNodes.forEach((node) => {
    if (String(node.data("backgroundImages")).includes("/device-")) {
      relayPlatformIconCount += 1;
    }
  });
  const positions: string[] = [];
  const edgeRates: string[] = [];
  const virtualPositions: string[] = [];
  const routedEdges: string[] = [];
  const edgeHitTargets: Array<{
    source: string;
    target: string;
    x: number;
    y: number;
  }> = [];
  const routes: RoutedPath[] = [];
  cy.nodes("[persistable]").forEach((node) => {
    const position = node.position();
    positions.push(
      `${node.id()}:${position.x.toFixed(2)},${position.y.toFixed(2)}`,
    );
  });
  positions.sort();
  cy.edges().forEach((edge) => {
    edgeRates.push(
      `${edge.id()}:${Number(edge.data("trafficWidth")).toFixed(4)}:${String(edge.data("label"))}`,
    );
    if (edge.scratch("tailpathObstacleRouted")) routedEdges.push(edge.id());
    const route = edge.scratch("tailpathRoute") as RoutedPath | undefined;
    if (route) routes.push(route);
    const midpoint = edge.midpoint();
    edgeHitTargets.push({
      source: edge.source().id(),
      target: edge.target().id(),
      x: Number(midpoint.x.toFixed(2)),
      y: Number(midpoint.y.toFixed(2)),
    });
  });
  cy.nodes().forEach((node) => {
    if (node.data("persistable")) return;
    const position = node.position();
    virtualPositions.push(
      `${node.id()}:${position.x.toFixed(2)},${position.y.toFixed(2)}`,
    );
  });
  edgeRates.sort();
  routedEdges.sort();
  virtualPositions.sort();
  const pan = cy.pan();
  element.dataset.deviceNodeCount = String(deviceNodes.length);
  element.dataset.deviceNodesSquare = String(deviceNodesSquare);
  element.dataset.relayPlatformIconCount = String(relayPlatformIconCount);
  element.dataset.layoutPositions = positions.join("|");
  element.dataset.layoutRuns = String(runs);
  element.dataset.edgeRateSignature = String(stableHash(edgeRates.join("|")));
  element.dataset.edgeHitTargets = JSON.stringify(edgeHitTargets);
  element.dataset.routedEdges = routedEdges.join("|");
  element.dataset.edgeCrossingCount = String(countRouteCrossings(routes));
  element.dataset.virtualPositions = virtualPositions.join("|");
  element.dataset.viewport = `${cy.zoom().toFixed(4)}:${pan.x.toFixed(2)},${pan.y.toFixed(2)}`;
}

function countRouteCrossings(routes: RoutedPath[]) {
  let count = 0;
  for (let left = 0; left < routes.length; left += 1) {
    for (let right = left + 1; right < routes.length; right += 1) {
      const leftRoute = routes[left];
      const rightRoute = routes[right];
      if (
        leftRoute.logicalEdgeID === rightRoute.logicalEdgeID ||
        [...leftRoute.nodeIDs].some((id) => rightRoute.nodeIDs.has(id))
      ) {
        continue;
      }
      if (pathsCross(leftRoute.points, rightRoute.points)) count += 1;
    }
  }
  return count;
}

function stableHash(value: string): number {
  let result = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    result ^= value.charCodeAt(index);
    result = Math.imul(result, 16777619);
  }
  return result >>> 0;
}
