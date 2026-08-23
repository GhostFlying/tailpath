import { useEffect, useMemo, useRef } from "react";
import cytoscape, {
  type Core,
  type ElementDefinition,
  type StylesheetCSS,
} from "cytoscape";
import type { Topology } from "../api/types";
import {
  buildElements,
  edgeIdealLengthForWidth,
  type PathFilter,
} from "../lib/graph";

interface Props {
  topology: Topology;
  pathFilter: PathFilter;
  showRecent: boolean;
  query: string;
  selectedEdgeId: string | null;
  onSelectEdge: (edgeId: string | null) => void;
  onSelectNode: (nodeId: string | null) => void;
}

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
  const structure = useRef("");
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
    structure.current = "";
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
    return () => {
      if (graph.current === cy) graph.current = null;
      cy.destroy();
    };
  }, []);

  useEffect(() => {
    const cy = graph.current;
    if (!cy) return;
    const preparedElements = elements.map(withMeasuredIdealLength);
    const nextIDs = new Set(
      preparedElements.map((element) => String(element.data?.id)),
    );
    cy.elements().forEach((element) => {
      if (!nextIDs.has(element.id())) element.remove();
    });
    for (const definition of preparedElements) {
      const id = String(definition.data?.id);
      const existing = cy.getElementById(id);
      if (existing.length) {
        existing.data(definition.data ?? {});
        existing.classes(definition.classes?.toString() ?? "");
      } else {
        cy.add(definition);
      }
    }
    const signature = preparedElements
      .map((element) => `${element.group}:${String(element.data?.id)}`)
      .sort()
      .join("|");
    if (signature !== structure.current) {
      const isInitialLayout = structure.current === "";
      if (container.current) container.current.dataset.ready = "false";
      structure.current = signature;
      const fit = () => {
        if (graph.current !== cy || cy.destroyed()) return;
        enforceEdgeClearance(cy);
        cy.resize();
        if (cy.nodes().length > 0) {
          cy.fit(cy.elements(), window.innerWidth <= 620 ? 28 : 72);
        }
        if (container.current) {
          const deviceNodes = cy.nodes(".device-node");
          let deviceNodesSquare = true;
          deviceNodes.forEach((node) => {
            if (!node.isNode() || node.width() !== 52 || node.height() !== 52) {
              deviceNodesSquare = false;
            }
          });
          container.current.dataset.deviceNodeCount = String(
            deviceNodes.length,
          );
          container.current.dataset.deviceNodesSquare =
            String(deviceNodesSquare);
          container.current.dataset.ready = "true";
        }
      };
      const layout = cy.layout({
        name: "cose",
        animate: false,
        randomize: isInitialLayout,
        fit: false,
        padding: 64,
        nodeRepulsion: () => 180000,
        idealEdgeLength: (edge) => edge.data("idealLength") as number,
        edgeElasticity: () => 80,
        gravity: 45,
        componentSpacing: 120,
      });
      layout.run();
      requestAnimationFrame(() => requestAnimationFrame(fit));
    } else if (enforceEdgeClearance(cy)) {
      cy.fit(cy.elements(), window.innerWidth <= 620 ? 28 : 72);
    }
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

  return (
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

function enforceEdgeClearance(cy: Core): boolean {
  let changed = false;
  for (let pass = 0; pass < 16; pass += 1) {
    let adjustedThisPass = false;
    cy.edges().forEach((edge, index) => {
      const source = edge.source();
      const target = edge.target();
      const sourcePosition = source.position();
      const targetPosition = target.position();
      let deltaX = targetPosition.x - sourcePosition.x;
      let deltaY = targetPosition.y - sourcePosition.y;
      let distance = Math.hypot(deltaX, deltaY);
      const minimumDistance = Number(edge.data("idealLength"));
      if (!Number.isFinite(minimumDistance) || distance >= minimumDistance) {
        return;
      }
      if (distance < 0.001) {
        const angle = ((index + pass) * Math.PI) / 4;
        deltaX = Math.cos(angle);
        deltaY = Math.sin(angle);
        distance = 1;
      }
      const displacement = (minimumDistance - distance) / 2;
      const offsetX = (deltaX / distance) * displacement;
      const offsetY = (deltaY / distance) * displacement;
      source.position({
        x: sourcePosition.x - offsetX,
        y: sourcePosition.y - offsetY,
      });
      target.position({
        x: targetPosition.x + offsetX,
        y: targetPosition.y + offsetY,
      });
      adjustedThisPass = true;
      changed = true;
    });
    if (!adjustedThisPass) break;
  }
  return changed;
}
