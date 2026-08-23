import type {
  EdgeHistory,
  PathEvent,
  PathKind,
  TrafficBucket,
} from "../api/types";

export interface ChartPoint {
  at: string;
  aToBRate: number;
  bToARate: number;
  x: number;
  aY: number;
  bY: number;
}

export interface TrafficGeometry {
  points: ChartPoint[];
  aLine: string;
  bLine: string;
  aArea: string;
  bArea: string;
  maxRate: number;
}

export interface PathTimelineItem {
  id: string;
  from: string;
  to: string;
  durationMs: number;
  path: PathEvent["path"];
  observations: PathEvent["observations"];
  anchored: boolean;
}

export function trafficGeometry(
  traffic: TrafficBucket[],
  bucketDurationMs: number,
  width = 900,
  height = 260,
): TrafficGeometry {
  const seconds = Math.max(0.001, bucketDurationMs / 1000);
  const rates = traffic.map((bucket) => ({
    at: bucket.bucketStart,
    aToBRate: Math.max(0, bucket.aToBBytes / seconds),
    bToARate: Math.max(0, bucket.bToABytes / seconds),
  }));
  const maxRate = rates.reduce(
    (maximum, point) => Math.max(maximum, point.aToBRate, point.bToARate),
    0,
  );
  const zero = height / 2;
  const amplitude = zero - 18;
  const denominator = Math.max(1, maxRate);
  const points = rates.map((point, index) => ({
    ...point,
    x: rates.length < 2 ? width / 2 : (index / (rates.length - 1)) * width,
    aY: zero - (point.aToBRate / denominator) * amplitude,
    bY: zero + (point.bToARate / denominator) * amplitude,
  }));
  const aLine = linePath(points.map((point) => [point.x, point.aY]));
  const bLine = linePath(points.map((point) => [point.x, point.bY]));
  return {
    points,
    aLine,
    bLine,
    aArea: areaPath(
      points.map((point) => [point.x, point.aY]),
      zero,
    ),
    bArea: areaPath(
      points.map((point) => [point.x, point.bY]),
      zero,
    ),
    maxRate,
  };
}

export function buildPathTimeline(history: EdgeHistory): PathTimelineItem[] {
  const events: Array<{ event: PathEvent; anchored: boolean }> = [];
  if (history.pathAnchor) {
    events.push({ event: history.pathAnchor, anchored: true });
  }
  events.push(
    ...history.pathEvents.map((event) => ({ event, anchored: false })),
  );
  if (events.length === 0) return [];
  return events.map(({ event, anchored }, index) => {
    const from = anchored ? history.from : event.observedAt;
    const to = events[index + 1]?.event.observedAt ?? history.to;
    return {
      id: `${event.observedAt}:${index}`,
      from,
      to,
      durationMs: Math.max(
        0,
        new Date(to).getTime() - new Date(from).getTime(),
      ),
      path: event.path,
      observations: event.observations,
      anchored,
    };
  });
}

export function pathColor(kind: PathKind): string {
  switch (kind) {
    case "direct":
      return "#16877a";
    case "derp":
      return "#bd7b00";
    case "peer_relay":
      return "#a4488e";
    default:
      return "#7f8a91";
  }
}

function linePath(points: Array<[number, number]>): string {
  return points
    .map(
      ([x, y], index) =>
        `${index === 0 ? "M" : "L"}${x.toFixed(2)},${y.toFixed(2)}`,
    )
    .join(" ");
}

function areaPath(points: Array<[number, number]>, zero: number): string {
  if (points.length === 0) return "";
  const first = points[0];
  const last = points[points.length - 1];
  const curve = points
    .map(([x, y]) => `L${x.toFixed(2)},${y.toFixed(2)}`)
    .join(" ");
  return `M${first[0].toFixed(2)},${zero.toFixed(2)} ${curve} L${last[0].toFixed(2)},${zero.toFixed(2)} Z`;
}
