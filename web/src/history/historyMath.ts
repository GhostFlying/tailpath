import type {
  DirectionalPathState,
  EdgeHistory,
  PathEvent,
  PathKind,
  TrafficBucket,
} from "../api/types";
import { pathIdentityKey } from "../lib/pathIdentity";

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
  observedAt: string;
  from: string;
  to: string;
  durationMs: number;
  path: PathEvent["path"];
  pathState: PathEvent["pathState"];
  pathCandidates: PathEvent["pathCandidates"];
  conflicts: PathEvent["conflicts"];
  observations: PathEvent["observations"];
  anchored: boolean;
}

export interface DirectionalTimelineSegment {
  id: string;
  observedAt: string;
  from: string;
  to: string;
  durationMs: number;
  event: PathEvent;
  aToB?: DirectionalPathState;
  bToA?: DirectionalPathState;
  anchored: boolean;
  noEvidence: boolean;
  coalescedCount?: number;
  coalescedMixed?: boolean;
  sourceSegmentId?: string;
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
  const eventIDs = stablePathEventIDs(events);
  const chronological = events.map(({ event, anchored }, index) => {
    const from = anchored ? history.from : event.observedAt;
    const to = events[index + 1]?.event.observedAt ?? history.to;
    return {
      id: eventIDs[index],
      observedAt: event.observedAt,
      from,
      to,
      durationMs: Math.max(
        0,
        new Date(to).getTime() - new Date(from).getTime(),
      ),
      path: event.path,
      pathState: event.pathState,
      pathCandidates: event.pathCandidates,
      conflicts: event.conflicts,
      observations: event.observations,
      anchored,
    };
  });
  return chronological.reverse();
}

export function buildDirectionalTimeline(
  history: EdgeHistory,
): DirectionalTimelineSegment[] {
  const events: Array<{
    event: PathEvent;
    anchored: boolean;
    noEvidence: boolean;
  }> = [];
  if (history.pathAnchor) {
    events.push({
      event: history.pathAnchor,
      anchored: true,
      noEvidence: false,
    });
  } else if (history.pathEvents.length) {
    events.push({
      event: {
        observedAt: history.from,
        path: { kind: "unknown" },
        conflicts: [],
        observations: [],
        directions: [],
      },
      anchored: true,
      noEvidence: true,
    });
  }
  events.push(
    ...history.pathEvents.map((event) => ({
      event,
      anchored: false,
      noEvidence: false,
    })),
  );
  if (!events.length) return [];
  const ordered = events.sort(
    (left, right) =>
      new Date(left.event.observedAt).getTime() -
      new Date(right.event.observedAt).getTime(),
  );
  const eventIDs = stablePathEventIDs(ordered);
  let hasSeenDirectionalEvent = false;
  return ordered.map(({ event, anchored, noEvidence }, index) => {
    const from = anchored
      ? history.from
      : clampTime(event.observedAt, history.from, history.to);
    const to = ordered[index + 1]
      ? clampTime(ordered[index + 1].event.observedAt, history.from, history.to)
      : history.to;
    const directions = event.directions ?? [];
    const eventIsDirectional = event.directions !== undefined;
    const eventHasDirections = directions.length > 0;
    const missingAfterDirectionalEvidence =
      !eventIsDirectional && hasSeenDirectionalEvent;
    hasSeenDirectionalEvent ||= eventIsDirectional;
    return {
      id: eventIDs[index],
      observedAt: event.observedAt,
      from,
      to,
      durationMs: Math.max(
        0,
        new Date(to).getTime() - new Date(from).getTime(),
      ),
      event,
      aToB: directions.find(
        (direction) =>
          direction.fromNodeId === history.source.id &&
          direction.toNodeId === history.target.id,
      ),
      bToA: directions.find(
        (direction) =>
          direction.fromNodeId === history.target.id &&
          direction.toNodeId === history.source.id,
      ),
      anchored,
      noEvidence:
        noEvidence ||
        (eventIsDirectional && !eventHasDirections) ||
        missingAfterDirectionalEvidence,
    };
  });
}

export function coalesceDirectionalTimeline(
  segments: DirectionalTimelineSegment[],
  limit: number,
): DirectionalTimelineSegment[] {
  const safeLimit = Math.max(1, Math.floor(limit));
  if (segments.length <= safeLimit) return segments;

  const timelineStart = new Date(segments[0].from).getTime();
  const timelineEnd = new Date(segments.at(-1)?.to ?? segments[0].to).getTime();
  if (
    !Number.isFinite(timelineStart) ||
    !Number.isFinite(timelineEnd) ||
    timelineEnd <= timelineStart
  ) {
    return [coalescedTimelineBin(segments, timelineStart, timelineEnd, 0)];
  }

  // Allocate the render budget by elapsed time, not event count. Count-based
  // chunks can make the final state in a burst appear to cover a much longer
  // preceding state. A mixed pixel bin is rendered as dense activity instead
  // of claiming that any one constituent state filled the entire interval.
  const durationMs = timelineEnd - timelineStart;
  const binCount = Math.min(safeLimit, Math.max(1, Math.ceil(durationMs)));
  const result: DirectionalTimelineSegment[] = [];
  let segmentIndex = 0;
  for (let binIndex = 0; binIndex < binCount; binIndex += 1) {
    const binStart =
      timelineStart + Math.floor((durationMs * binIndex) / binCount);
    const binEnd =
      timelineStart + Math.floor((durationMs * (binIndex + 1)) / binCount);
    while (
      segmentIndex < segments.length &&
      new Date(segments[segmentIndex].to).getTime() <= binStart
    ) {
      segmentIndex += 1;
    }
    const overlapping: DirectionalTimelineSegment[] = [];
    for (let index = segmentIndex; index < segments.length; index += 1) {
      const segment = segments[index];
      const segmentStart = new Date(segment.from).getTime();
      const segmentEnd = new Date(segment.to).getTime();
      if (segmentStart >= binEnd) break;
      if (segmentEnd > binStart) overlapping.push(segment);
    }
    if (overlapping.length) {
      result.push(
        coalescedTimelineBin(overlapping, binStart, binEnd, binIndex),
      );
    }
  }
  return result;
}

function coalescedTimelineBin(
  segments: DirectionalTimelineSegment[],
  fromMs: number,
  toMs: number,
  index: number,
): DirectionalTimelineSegment {
  const latest = segments.at(-1) ?? segments[0];
  const stateKeys = new Set(segments.map(directionalTimelineStateKey));
  return {
    ...latest,
    id: `render-bin:${index}:${latest.id}`,
    sourceSegmentId: latest.sourceSegmentId ?? latest.id,
    from: Number.isFinite(fromMs)
      ? new Date(fromMs).toISOString()
      : latest.from,
    to: Number.isFinite(toMs) ? new Date(toMs).toISOString() : latest.to,
    durationMs: Math.max(0, toMs - fromMs),
    coalescedCount: segments.length,
    coalescedMixed: stateKeys.size > 1,
  };
}

function directionalTimelineStateKey(
  segment: DirectionalTimelineSegment,
): string {
  const directionKey = (state: DirectionalPathState | undefined) =>
    state
      ? [
          state.fromNodeId,
          state.toNodeId,
          pathIdentityKey(state.primaryPath),
          state.fallbackPath ? pathIdentityKey(state.fallbackPath) : "",
          state.evidence,
          state.inferenceRule ?? "",
          state.observerId,
        ].join("\u001f")
      : "missing";
  return [
    segment.noEvidence ? "no-evidence" : "evidence",
    directionKey(segment.aToB),
    directionKey(segment.bToA),
    pathIdentityKey(segment.event.path),
    segment.event.pathState ?? "",
  ].join("\u001e");
}

function stablePathEventIDs(
  events: Array<{ event: PathEvent; anchored: boolean; noEvidence?: boolean }>,
): string[] {
  const ids = new Array<string>(events.length);
  const occurrencesFromNewest = new Map<string, number>();
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const item = events[index];
    const identity = `${item.anchored ? "anchor" : "event"}:${item.noEvidence ? "empty" : "evidence"}:${item.event.observedAt}:${compactEventDigest(item.event)}`;
    const occurrence = occurrencesFromNewest.get(identity) ?? 0;
    ids[index] = `${identity}:${occurrence}`;
    occurrencesFromNewest.set(identity, occurrence + 1);
  }
  return ids;
}

function compactEventDigest(event: PathEvent): string {
  const serialized = JSON.stringify(event);
  let first = 0x811c9dc5;
  let second = 0x9e3779b9;
  for (let index = 0; index < serialized.length; index += 1) {
    const code = serialized.charCodeAt(index);
    first = Math.imul(first ^ code, 0x01000193);
    second = Math.imul(second ^ code, 0x85ebca6b);
    second ^= second >>> 13;
  }
  return `${(first >>> 0).toString(16).padStart(8, "0")}${(second >>> 0).toString(16).padStart(8, "0")}`;
}

export function hasDirectionalHistory(history: EdgeHistory): boolean {
  return [history.pathAnchor, ...history.pathEvents].some(
    (event) => event?.directions !== undefined,
  );
}

export function pathEvidenceKey(path: PathEvent["path"]): string {
  return pathIdentityKey(path);
}

export function compatibilityPathEvidenceKey(path: PathEvent["path"]): string {
  switch (path.kind) {
    case "direct":
      return "direct";
    case "derp":
      return `derp:${path.derpRegion?.trim().toLowerCase() || "unknown"}`;
    case "peer_relay": {
      const stableID = path.peerRelayStableNodeId?.trim();
      if (stableID) return `peer_relay:${stableID}`;
      const endpoint = path.peerRelayEndpoint?.trim();
      if (endpoint) return `peer_relay:endpoint:${endpoint}`;
      if (path.peerRelayVni !== undefined) {
        return `peer_relay:vni:${path.peerRelayVni}`;
      }
      return "peer_relay:unknown";
    }
    default:
      return "unknown";
  }
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

function clampTime(value: string, from: string, to: string) {
  const timestamp = new Date(value).getTime();
  const start = new Date(from).getTime();
  const end = new Date(to).getTime();
  if (timestamp <= start) return from;
  if (timestamp >= end) return to;
  return value;
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
