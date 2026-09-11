/// <reference types="vite/client" />
import type { Core, EdgeSingular, NodeSingular } from "cytoscape";
import {
  center,
  centered,
  detailMode,
  expand,
  intersects,
  layoutTokens,
  placeLabel,
  pointAlong,
  quadratic,
  segmentHitsRect,
  shorten,
  SpatialIndex,
  type DetailMode,
  type Occupant,
  type Point,
  type Rect,
} from "./layoutGeometry";

const fontFamily = "Inter, ui-sans-serif, system-ui, sans-serif";
const fontCache = new Map<string, number>();
let context: CanvasRenderingContext2D | null = null;
function measure(text: string, size: number, weight = 600) {
  const font = `${weight} ${size}px ${fontFamily}`,
    key = `${font}:${text}`;
  const cached = fontCache.get(key);
  if (cached !== undefined) return cached;
  context ??= document.createElement("canvas").getContext("2d");
  if (!context) return text.length * size;
  context.font = font;
  const width = Math.ceil(context.measureText(text).width);
  if (fontCache.size >= 4096) fontCache.clear();
  fontCache.set(key, width);
  return width;
}
export function clearFontMeasurements() {
  fontCache.clear();
}
function body(node: NodeSingular): Rect {
  // The generated badges are contained by the body; reserve external stroke/focus too.
  return node.renderedBoundingBox({
    includeLabels: false,
    includeOverlays: true,
  });
}
function labelBounds(node: NodeSingular | EdgeSingular): Rect {
  return node.renderedBoundingBox({
    includeNodes: false,
    includeEdges: false,
    includeLabels: true,
    includeOverlays: false,
  });
}
function screen(cy: Core, point: Point): Point {
  return {
    x: point.x * cy.zoom() + cy.pan().x,
    y: point.y * cy.zoom() + cy.pan().y,
  };
}
export function renderedPath(cy: Core, edge: EdgeSingular): Point[] {
  const a = screen(cy, edge.sourceEndpoint()),
    b = screen(cy, edge.targetEndpoint());
  const controls = edge.controlPoints();
  if (controls?.length === 1) return quadratic(a, screen(cy, controls[0]), b);
  const segments = edge.segmentPoints();
  if (segments?.length) return [a, ...segments.map((p) => screen(cy, p)), b];
  if (controls?.length) {
    const points = [a];
    for (let i = 0; i < controls.length; i++) {
      const c = screen(cy, controls[i]);
      const next =
        i === controls.length - 1
          ? b
          : center(centered(screen(cy, controls[i + 1]), 0, 0));
      const end =
        i === controls.length - 1
          ? b
          : { x: (c.x + next.x) / 2, y: (c.y + next.y) / 2 };
      points.push(...quadratic(points.at(-1)!, c, end).slice(1));
    }
    return points;
  }
  return [a, b];
}
interface Route {
  id: string;
  logical: string;
  points: Point[];
  width: number;
}
export interface PresentationSummary {
  mode: DetailMode;
  hidden: number;
  collisions: number;
}
export interface PresentationOptions {
  reroute: (moveVirtual: boolean) => void;
  onComplete: (summary: PresentationSummary) => void;
}

/** Owns transient anchors and cancellation, never canonical position persistence. */
export function createLayoutPresentation(
  cy: Core,
  container: HTMLDivElement,
  options: PresentationOptions,
) {
  let mode: DetailMode = "detail",
    frame = 0,
    revision = 0;
  let anchors = new Map<string, number>();
  let disposed = false;
  function exclusions(): Rect[] {
    const canvas = container.getBoundingClientRect();
    return [
      ...document.querySelectorAll<HTMLElement>(
        ".graph-controls, .graph-objects[open], .inspector:not(.empty)",
      ),
    ]
      .filter((el) => el.getClientRects().length > 0)
      .map((el) => {
        const r = el.getBoundingClientRect();
        return {
          x1: r.left - canvas.left - 4,
          y1: r.top - canvas.top - 4,
          x2: r.right - canvas.left + 4,
          y2: r.bottom - canvas.top + 4,
        };
      });
  }
  function* solve(): Generator<void, PresentationSummary> {
    mode = detailMode(mode, cy.zoom());
    const zoom = cy.zoom();
    const nodes = cy
      .nodes()
      .sort(
        (a, b) =>
          Number(b.selected()) - Number(a.selected()) ||
          a.id().localeCompare(b.id()),
      );
    if (nodes.length > 80) mode = "overview";
    const important = new Set(
      cy
        .elements(":selected")
        .closedNeighborhood()
        .nodes()
        .map((n) => n.id()),
    );
    const occupied = new SpatialIndex<Occupant>(),
      blocked = exclusions();
    const texts = new Map<string, string>(),
      duplicates = new Map<string, number>();
    let hidden = 0;
    for (const node of nodes) {
      const full = String(node.data("label") ?? "");
      const text = shorten(full, layoutTokens.nameWidth, (s) => measure(s, 12));
      texts.set(node.id(), text);
      duplicates.set(text, (duplicates.get(text) ?? 0) + 1);
    }
    cy.startBatch();
    try {
      for (const node of nodes) {
        let text = texts.get(node.id())!;
        if (!node.hasClass("path-marker") && (duplicates.get(text) ?? 0) > 1) {
          const suffix = ` ·${node.id().slice(-6)}`;
          text =
            shorten(text, layoutTokens.nameWidth - measure(suffix, 12), (s) =>
              measure(s, 12),
            ) + suffix;
        }
        node.style({
          label: text,
          "font-size": 12 / zoom,
          "text-wrap": "none",
          "text-opacity": 1,
          "text-background-padding": `${3 / zoom}px`,
        });
        if (
          mode === "overview" &&
          nodes.length > 12 &&
          !important.has(node.id()) &&
          !node.hasClass("path-marker")
        )
          node.style("text-opacity", 0);
        if (node.hasClass("derp")) {
          node.style({
            width:
              Math.min(layoutTokens.relayWidth, measure(text, 12) + 24) / zoom,
            height: 34 / zoom,
            "text-valign": "center",
            "text-halign": "center",
            "text-margin-x": 0,
            "text-margin-y": 0,
          });
        }
        if (
          node.hasClass("derp") &&
          mode === "overview" &&
          nodes.length > 12 &&
          !important.has(node.id())
        ) {
          node.style({
            width: 18 / zoom,
            height: 14 / zoom,
            "text-opacity": 0,
          });
          hidden++;
        }
        if (node.hasClass("path-marker"))
          node.style({
            "font-size": 12 / zoom,
            width: 18 / zoom,
            height: 18 / zoom,
          });
      }
    } finally {
      cy.endBatch();
    }
    // Existing bounded routing also places virtual relays against measured labels.
    options.reroute(true);
    for (const node of nodes)
      occupied.add({ id: node.id(), bounds: body(node) });
    const nextAnchors = new Map<string, number>();
    const ordered = [...nodes].sort(
      (a, b) =>
        Number(important.has(b.id())) - Number(important.has(a.id())) ||
        Number(a.hasClass("recent")) - Number(b.hasClass("recent")) ||
        a.id().localeCompare(b.id()),
    );
    let visibleNames = 0;
    cy.startBatch();
    try {
      for (const node of ordered) {
        if (node.hasClass("derp") || node.hasClass("path-marker")) continue;
        if (
          !important.has(node.id()) &&
          ((mode === "overview" && nodes.length > 12) || visibleNames >= 80)
        ) {
          node.style("text-opacity", 0);
          hidden++;
          continue;
        }
        const text = String(node.style("label"));
        const request = {
          id: `name:${node.id()}`,
          owner: node.id(),
          body: body(node),
          width: measure(text, 12) + 8,
          height: 22,
          previous: anchors.get(node.id()),
          priority: important.has(node.id()) ? 0 : 1,
        };
        const placement = placeLabel(request, occupied, blocked);
        if (!placement) {
          node.style("text-opacity", 0);
          hidden++;
          continue;
        }
        const p = center(placement.bounds),
          origin = node.renderedPosition();
        node.style({
          "text-valign": "center",
          "text-halign": "center",
          "text-margin-x": (p.x - origin.x) / zoom,
          "text-margin-y": (p.y - origin.y) / zoom,
        });
        occupied.add({ id: request.id, bounds: placement.bounds });
        nextAnchors.set(node.id(), placement.anchor);
        visibleNames++;
      }
    } finally {
      cy.endBatch();
    }
    anchors = nextAnchors;
    // Re-evaluate routes against final name anchors before placing rate labels.
    options.reroute(false);
    const needsRoutes = mode !== "overview" || cy.edges(":selected").length > 0;
    const routes: Route[] = (
      needsRoutes || import.meta.env.VITE_LAYOUT_DIAGNOSTICS === "1"
        ? cy.edges()
        : cy.collection()
    ).map((e) => ({
      id: e.id(),
      logical: String(e.data("logicalEdgeId")),
      points: renderedPath(cy, e),
      width: e.renderedWidth(),
    }));
    const routeIndex = new SpatialIndex<
      Occupant & { route: Route; a: Point; b: Point }
    >();
    for (const route of needsRoutes ? routes : [])
      for (let i = 1; i < route.points.length; i++) {
        const a = route.points[i - 1],
          b = route.points[i];
        routeIndex.add({
          id: route.id,
          route,
          a,
          b,
          bounds: expand(
            {
              x1: Math.min(a.x, b.x),
              x2: Math.max(a.x, b.x),
              y1: Math.min(a.y, b.y),
              y2: Math.max(a.y, b.y),
            },
            route.width / 2 + 1,
          ),
        });
      }
    const routeMap = new Map(routes.map((r) => [r.id, r]));
    cy.batch(() =>
      cy.edges().style({
        "font-size": 11 / zoom,
        "text-background-padding": `${3 / zoom}px`,
        "text-opacity": 0,
      }),
    );
    const shown = new Set<string>();
    for (const edge of cy
      .edges()
      .sort(
        (a, b) =>
          Number(b.selected()) - Number(a.selected()) ||
          a.id().localeCompare(b.id()),
      )) {
      const text = String(edge.data("label") ?? ""),
        logical = String(edge.data("logicalEdgeId"));
      if (!text || edge.hasClass("recent")) continue;
      if (shown.has(logical)) continue;
      shown.add(logical);
      if (mode === "overview" && !edge.selected()) {
        hidden++;
        yield;
        continue;
      }
      const route = routeMap.get(edge.id())!;
      let placed = false;
      for (const fraction of [0.5, 0.35, 0.65]) {
        const p = pointAlong(route.points, fraction),
          bounds = centered(p, measure(text, 11, 400) + 8, 21);
        const protectedBounds = expand(bounds, layoutTokens.pathGap);
        if (
          blocked.some((r) => intersects(r, bounds)) ||
          occupied.query(expand(bounds, layoutTokens.labelGap)).length
        )
          continue;
        if (
          routeIndex
            .query(protectedBounds)
            .some(
              (r) =>
                r.id !== edge.id() &&
                segmentHitsRect(
                  r.a,
                  r.b,
                  expand(bounds, layoutTokens.pathGap + r.route.width / 2 + 1),
                ),
            )
        )
          continue;
        const start = route.points[0],
          end = route.points.at(-1)!;
        if (
          [start, end].some((endpoint) =>
            intersects(expand(bounds, 8), centered(endpoint, 14, 14)),
          )
        )
          continue;
        const mid = screen(cy, edge.midpoint());
        edge.style({
          "text-opacity": 1,
          "text-margin-x": (p.x - mid.x) / zoom,
          "text-margin-y": (p.y - mid.y) / zoom,
        });
        occupied.add({
          id: `rate:${edge.id()}`,
          bounds: expand(labelBounds(edge), 1),
        });
        placed = true;
        break;
      }
      if (!placed) hidden++;
      yield;
    }
    let collisions = 0;
    const bodyIndex = new SpatialIndex<Occupant>();
    for (const node of nodes) {
      const bounds = body(node);
      collisions += bodyIndex.query(
        expand(bounds, layoutTokens.bodyGap),
      ).length;
      bodyIndex.add({ id: node.id(), bounds });
    }
    if (import.meta.env.VITE_LAYOUT_DIAGNOSTICS === "1") {
      container.dataset.geometry = JSON.stringify({
        width: cy.width(),
        height: cy.height(),
        mode,
        hidden,
        collisions,
        bodies: nodes.map((n) => ({
          id: n.id(),
          bounds: body(n as NodeSingular),
          canonical: Boolean(n.data("persistable")),
        })),
        labels: [...nodes, ...cy.edges()]
          .filter(
            (e) =>
              Number(e.style("text-opacity")) > 0 && String(e.style("label")),
          )
          .map((e) => ({
            id: e.id(),
            owner: e.id(),
            kind: e.isNode() ? "name" : "rate",
            internal: e.hasClass("derp") || e.hasClass("path-marker"),
            bounds: labelBounds(e),
          })),
        routes,
      });
    }
    return { mode, hidden, collisions };
  }
  function request() {
    if (disposed || cy.destroyed()) return;
    const current = ++revision;
    cancelAnimationFrame(frame);
    container.dataset.presentationReady = "false";
    const job = solve();
    function step() {
      if (disposed || cy.destroyed() || current !== revision) return;
      const start = performance.now();
      let result: IteratorResult<void, PresentationSummary>;
      do {
        result = job.next();
      } while (
        !result.done &&
        performance.now() - start < layoutTokens.frameBudget
      );
      if (result.done) {
        container.dataset.presentationReady = "true";
        container.dataset.detailMode = result.value.mode;
        options.onComplete(result.value);
      } else frame = requestAnimationFrame(step);
    }
    frame = requestAnimationFrame(step);
  }
  return {
    request,
    dispose() {
      disposed = true;
      revision++;
      cancelAnimationFrame(frame);
    },
  };
}
