import { describe, expect, it } from "vitest";
import type { EdgeHistory } from "../api/types";
import {
  buildPathTimeline,
  trafficGeometry,
  trafficPointAtX,
} from "./historyMath";

describe("history chart geometry", () => {
  it("keeps rates non-negative while mirroring only SVG coordinates", () => {
    const geometry = trafficGeometry(
      [
        { bucketStart: "2026-08-24T00:00:00Z", aToBBytes: 100, bToABytes: 50 },
        { bucketStart: "2026-08-24T00:00:10Z", aToBBytes: 20, bToABytes: 80 },
      ],
      10_000,
      "2026-08-24T00:00:00Z",
      "2026-08-24T00:00:20Z",
    );
    expect(
      geometry.points.every(
        (point) => point.aToBRate >= 0 && point.bToARate >= 0,
      ),
    ).toBe(true);
    expect(
      geometry.points.every((point) => point.aY <= 130 && point.bY >= 130),
    ).toBe(true);
    expect(geometry.aArea.match(/M/g)).toHaveLength(1);
    expect(geometry.bArea.match(/M/g)).toHaveLength(1);
  });

  it("positions sparse buckets by timestamp and leaves gaps unconnected", () => {
    const geometry = trafficGeometry(
      [
        { bucketStart: "2026-08-24T00:50:00Z", aToBBytes: 100, bToABytes: 20 },
        { bucketStart: "2026-08-24T00:10:00Z", aToBBytes: 50, bToABytes: 10 },
        { bucketStart: "2026-08-23T23:00:00Z", aToBBytes: 999, bToABytes: 999 },
      ],
      10_000,
      "2026-08-24T00:00:00Z",
      "2026-08-24T01:00:00Z",
    );

    expect(geometry.points).toHaveLength(2);
    expect(geometry.points[0].x).toBeCloseTo(151.25, 2);
    expect(geometry.points[1].x).toBeCloseTo(751.25, 2);
    expect(geometry.aLine.match(/M/g)).toHaveLength(2);
    expect(geometry.aArea.match(/M/g)).toHaveLength(2);
    expect(trafficPointAtX(geometry.points, geometry.points[0].x)).toBe(0);
    expect(trafficPointAtX(geometry.points, 450)).toBeNull();
  });

  it("renders adjacent buckets as one step run", () => {
    const geometry = trafficGeometry(
      [
        { bucketStart: "2026-08-24T00:00:10Z", aToBBytes: 10, bToABytes: 20 },
        { bucketStart: "2026-08-24T00:00:00Z", aToBBytes: 20, bToABytes: 10 },
      ],
      10_000,
      "2026-08-24T00:00:00Z",
      "2026-08-24T00:00:20Z",
    );

    expect(geometry.aLine.match(/M/g)).toHaveLength(1);
    expect(geometry.aLine.match(/L/g)).toHaveLength(3);
    expect(geometry.points.map((point) => point.at)).toEqual([
      "2026-08-24T00:00:00Z",
      "2026-08-24T00:00:10Z",
    ]);
  });
});

describe("path timeline", () => {
  it("starts an anchor at the selected window boundary", () => {
    const history = {
      edgeId: "a--b",
      source: { id: "a", label: "A" },
      target: { id: "b", label: "B" },
      systemTelemetry: false,
      relatedNodes: [],
      from: "2026-08-24T00:00:00Z",
      to: "2026-08-24T01:00:00Z",
      bucketDurationMs: 30_000,
      traffic: [],
      pathAnchor: {
        observedAt: "2026-08-23T23:00:00Z",
        path: { kind: "direct" },
        conflicts: [],
        observations: [],
      },
      pathEvents: [
        {
          observedAt: "2026-08-24T00:30:00Z",
          path: { kind: "derp", derpRegion: "hkg" },
          conflicts: [],
          observations: [],
        },
      ],
      trafficTruncated: false,
      pathEventsTruncated: false,
    } satisfies EdgeHistory;
    const timeline = buildPathTimeline(history);
    expect(timeline).toHaveLength(2);
    expect(timeline[0]).toMatchObject({
      from: history.from,
      to: history.pathEvents[0].observedAt,
      anchored: true,
    });
    expect(timeline[1].durationMs).toBe(30 * 60 * 1000);
  });
});
