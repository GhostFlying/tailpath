/** Screen-space geometry. No renderer, DOM, or Tailnet types belong here. */
export interface Point {
  x: number;
  y: number;
}
export interface Rect {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}
export const layoutTokens = {
  bodyGap: 12,
  labelGap: 8,
  pathGap: 4,
  viewportGap: 16,
  nameWidth: 160,
  relayWidth: 184,
  nameFont: 12,
  rateFont: 11,
  frameBudget: 8,
  overviewEnter: 0.65,
  overviewExit: 0.8,
} as const;
export type DetailMode = "detail" | "overview";
export function detailMode(previous: DetailMode, zoom: number): DetailMode {
  if (zoom < layoutTokens.overviewEnter) return "overview";
  if (zoom > layoutTokens.overviewExit) return "detail";
  return previous;
}
export function expand(r: Rect, amount: number): Rect {
  return {
    x1: r.x1 - amount,
    y1: r.y1 - amount,
    x2: r.x2 + amount,
    y2: r.y2 + amount,
  };
}
export function intersects(a: Rect, b: Rect): boolean {
  return a.x1 < b.x2 && a.x2 > b.x1 && a.y1 < b.y2 && a.y2 > b.y1;
}
export function contains(outer: Rect, inner: Rect): boolean {
  return (
    inner.x1 >= outer.x1 &&
    inner.y1 >= outer.y1 &&
    inner.x2 <= outer.x2 &&
    inner.y2 <= outer.y2
  );
}
export function centered(p: Point, width: number, height: number): Rect {
  return {
    x1: p.x - width / 2,
    y1: p.y - height / 2,
    x2: p.x + width / 2,
    y2: p.y + height / 2,
  };
}
export function center(r: Rect): Point {
  return { x: (r.x1 + r.x2) / 2, y: (r.y1 + r.y2) / 2 };
}

/** Bounded cells, with a separate bucket for unusually long routes. */
export class SpatialIndex<T extends { bounds: Rect }> {
  private cells = new Map<string, T[]>();
  private broad: T[] = [];
  private all: T[] = [];
  private keys(r: Rect): string[] | null {
    const x1 = Math.floor(r.x1 / 96),
      x2 = Math.floor(r.x2 / 96);
    const y1 = Math.floor(r.y1 / 96),
      y2 = Math.floor(r.y2 / 96);
    if (![x1, x2, y1, y2].every(Number.isSafeInteger)) return null;
    if ((x2 - x1 + 1) * (y2 - y1 + 1) > 256) return null;
    const keys: string[] = [];
    for (let x = x1; x <= x2; x++)
      for (let y = y1; y <= y2; y++) keys.push(`${x}:${y}`);
    return keys;
  }
  add(item: T) {
    this.all.push(item);
    const keys = this.keys(item.bounds);
    if (!keys) {
      this.broad.push(item);
      return;
    }
    for (const key of keys) {
      const values = this.cells.get(key) ?? [];
      values.push(item);
      this.cells.set(key, values);
    }
  }
  query(bounds: Rect): T[] {
    const keys = this.keys(bounds);
    const candidates = keys
      ? [...this.broad, ...keys.flatMap((key) => this.cells.get(key) ?? [])]
      : this.all;
    return [...new Set(candidates)].filter((item) =>
      intersects(bounds, item.bounds),
    );
  }
}

export interface Occupant {
  id: string;
  bounds: Rect;
}
export interface LabelRequest {
  id: string;
  owner: string;
  body: Rect;
  width: number;
  height: number;
  previous?: number;
  priority: number;
}
export interface LabelPlacement {
  id: string;
  owner: string;
  bounds: Rect;
  anchor: number;
}
export function labelCandidates(
  body: Rect,
  width: number,
  height: number,
): Rect[] {
  const c = center(body),
    gap = layoutTokens.labelGap;
  const below = body.y2 + gap + height / 2,
    above = body.y1 - gap - height / 2;
  const left = body.x1 - gap - width / 2,
    right = body.x2 + gap + width / 2;
  return [
    { x: c.x, y: below },
    { x: c.x, y: above },
    { x: right, y: c.y },
    { x: left, y: c.y },
    { x: right, y: below },
    { x: left, y: below },
    { x: right, y: above },
    { x: left, y: above },
  ].map((p) => centered(p, width, height));
}
export function placeLabel(
  request: LabelRequest,
  occupied: SpatialIndex<Occupant>,
  exclusions: Rect[],
): LabelPlacement | null {
  const candidates = labelCandidates(
    request.body,
    request.width,
    request.height,
  );
  const order = [...new Set([request.previous ?? 0, 0, 1, 2, 3, 4, 5, 6, 7])];
  for (const anchor of order) {
    const bounds = candidates[anchor];
    if (!bounds || exclusions.some((r) => intersects(r, bounds))) continue;
    if (occupied.query(expand(bounds, layoutTokens.labelGap)).length) continue;
    return { id: request.id, owner: request.owner, bounds, anchor };
  }
  return null;
}

export function segmentHitsRect(a: Point, b: Point, r: Rect): boolean {
  let low = 0,
    high = 1;
  for (const [start, delta, min, max] of [
    [a.x, b.x - a.x, r.x1, r.x2],
    [a.y, b.y - a.y, r.y1, r.y2],
  ]) {
    if (Math.abs(delta) < 1e-9) {
      if (start < min || start > max) return false;
      continue;
    }
    const first = (min - start) / delta,
      last = (max - start) / delta;
    low = Math.max(low, Math.min(first, last));
    high = Math.min(high, Math.max(first, last));
    if (low > high) return false;
  }
  return true;
}
/** Subdivide a quadratic until control-to-chord error is <= one CSS pixel. */
export function quadratic(
  a: Point,
  control: Point,
  b: Point,
  tolerance = 1,
): Point[] {
  const result = [a];
  function split(start: Point, c: Point, end: Point, depth: number) {
    const dx = end.x - start.x,
      dy = end.y - start.y;
    const distance = Math.hypot(dx, dy);
    const error =
      distance < 1e-9
        ? Math.hypot(c.x - start.x, c.y - start.y)
        : Math.abs(dy * c.x - dx * c.y + end.x * start.y - end.y * start.x) /
          distance;
    if (error <= tolerance || depth >= 12) {
      result.push(end);
      return;
    }
    const first = { x: (start.x + c.x) / 2, y: (start.y + c.y) / 2 };
    const second = { x: (c.x + end.x) / 2, y: (c.y + end.y) / 2 };
    const mid = { x: (first.x + second.x) / 2, y: (first.y + second.y) / 2 };
    split(start, first, mid, depth + 1);
    split(mid, second, end, depth + 1);
  }
  split(a, control, b, 0);
  return result;
}
export function pointAlong(points: Point[], fraction: number): Point {
  const lengths = points
    .slice(1)
    .map((p, i) => Math.hypot(p.x - points[i].x, p.y - points[i].y));
  let remaining = lengths.reduce((a, b) => a + b, 0) * fraction;
  for (let i = 0; i < lengths.length; i++) {
    if (remaining <= lengths[i] && lengths[i] > 0) {
      const t = remaining / lengths[i];
      return {
        x: points[i].x + (points[i + 1].x - points[i].x) * t,
        y: points[i].y + (points[i + 1].y - points[i].y) * t,
      };
    }
    remaining -= lengths[i];
  }
  return points.at(-1) ?? { x: 0, y: 0 };
}

export function shorten(
  text: string,
  maxWidth: number,
  measure: (text: string) => number,
): string {
  if (measure(text) <= maxWidth) return text;
  const chars = Array.from(text);
  let low = 0,
    high = chars.length;
  while (low < high) {
    const n = Math.ceil((low + high) / 2),
      tail = Math.ceil(n / 2);
    const candidate =
      chars.slice(0, n - tail).join("") +
      "…" +
      (tail ? chars.slice(-tail).join("") : "");
    if (measure(candidate) <= maxWidth) low = n;
    else high = n - 1;
  }
  const tail = Math.ceil(low / 2);
  return (
    chars.slice(0, low - tail).join("") +
    "…" +
    (tail ? chars.slice(-tail).join("") : "")
  );
}
