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
  xStart: number;
  xEnd: number;
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
  from: string,
  to: string,
  width = 900,
  height = 260,
): TrafficGeometry {
  const fromMs = new Date(from).getTime();
  const toMs = new Date(to).getTime();
  if (!Number.isFinite(fromMs) || !Number.isFinite(toMs) || toMs <= fromMs) {
    return emptyTrafficGeometry();
  }
  const seconds = Math.max(0.001, bucketDurationMs / 1000);
  const durationMs = Math.max(1, bucketDurationMs);
  const rates = traffic
    .flatMap((bucket) => {
      const bucketStartMs = new Date(bucket.bucketStart).getTime();
      const bucketEndMs = bucketStartMs + durationMs;
      const visibleStartMs = Math.max(fromMs, bucketStartMs);
      const visibleEndMs = Math.min(toMs, bucketEndMs);
      if (!Number.isFinite(bucketStartMs) || visibleEndMs <= visibleStartMs) {
        return [];
      }
      return [
        {
          at: bucket.bucketStart,
          startMs: visibleStartMs,
          endMs: visibleEndMs,
          aToBRate: Math.max(0, bucket.aToBBytes / seconds),
          bToARate: Math.max(0, bucket.bToABytes / seconds),
        },
      ];
    })
    .sort((left, right) => left.startMs - right.startMs);
  const maxRate = rates.reduce(
    (maximum, point) => Math.max(maximum, point.aToBRate, point.bToARate),
    0,
  );
  const zero = height / 2;
  const amplitude = zero - 18;
  const denominator = Math.max(1, maxRate);
  const points = rates.map((point) => {
    const xStart = ((point.startMs - fromMs) / (toMs - fromMs)) * width;
    const xEnd = ((point.endMs - fromMs) / (toMs - fromMs)) * width;
    return {
      at: point.at,
      aToBRate: point.aToBRate,
      bToARate: point.bToARate,
      x: (xStart + xEnd) / 2,
      xStart,
      xEnd,
      aY: zero - (point.aToBRate / denominator) * amplitude,
      bY: zero + (point.bToARate / denominator) * amplitude,
    };
  });
  return {
    points,
    aLine: stepLinePath(points, "aY"),
    bLine: stepLinePath(points, "bY"),
    aArea: stepAreaPath(points, "aY", zero),
    bArea: stepAreaPath(points, "bY", zero),
    maxRate,
  };
}

export function trafficPointAtX(
  points: ChartPoint[],
  x: number,
): number | null {
  const index = points.findIndex(
    (point, pointIndex) =>
      x >= point.xStart &&
      (x < point.xEnd || (pointIndex === points.length - 1 && x <= point.xEnd)),
  );
  return index < 0 ? null : index;
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

function emptyTrafficGeometry(): TrafficGeometry {
  return {
    points: [],
    aLine: "",
    bLine: "",
    aArea: "",
    bArea: "",
    maxRate: 0,
  };
}

function stepLinePath(points: ChartPoint[], key: "aY" | "bY"): string {
  return trafficRuns(points)
    .map((run) => {
      const first = run[0];
      const commands = [
        `M${first.xStart.toFixed(2)},${first[key].toFixed(2)}`,
        `L${first.xEnd.toFixed(2)},${first[key].toFixed(2)}`,
      ];
      for (const point of run.slice(1)) {
        commands.push(
          `L${point.xStart.toFixed(2)},${point[key].toFixed(2)}`,
          `L${point.xEnd.toFixed(2)},${point[key].toFixed(2)}`,
        );
      }
      return commands.join(" ");
    })
    .join(" ");
}

function stepAreaPath(
  points: ChartPoint[],
  key: "aY" | "bY",
  zero: number,
): string {
  return trafficRuns(points)
    .map((run) => {
      const first = run[0];
      const last = run[run.length - 1];
      const commands = [
        `M${first.xStart.toFixed(2)},${zero.toFixed(2)}`,
        `L${first.xStart.toFixed(2)},${first[key].toFixed(2)}`,
        `L${first.xEnd.toFixed(2)},${first[key].toFixed(2)}`,
      ];
      for (const point of run.slice(1)) {
        commands.push(
          `L${point.xStart.toFixed(2)},${point[key].toFixed(2)}`,
          `L${point.xEnd.toFixed(2)},${point[key].toFixed(2)}`,
        );
      }
      commands.push(`L${last.xEnd.toFixed(2)},${zero.toFixed(2)} Z`);
      return commands.join(" ");
    })
    .join(" ");
}

function trafficRuns(points: ChartPoint[]): ChartPoint[][] {
  const runs: ChartPoint[][] = [];
  for (const point of points) {
    const current = runs.at(-1);
    const previous = current?.at(-1);
    if (
      !current ||
      !previous ||
      Math.abs(previous.xEnd - point.xStart) > 0.01
    ) {
      runs.push([point]);
    } else {
      current.push(point);
    }
  }
  return runs;
}
