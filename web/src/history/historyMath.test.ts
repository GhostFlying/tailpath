import { describe, expect, it } from "vitest";
import type { EdgeHistory } from "../api/types";
import { buildPathTimeline, trafficGeometry } from "./historyMath";

describe("history chart geometry", () => {
  it("keeps rates non-negative while mirroring only SVG coordinates", () => {
    const geometry = trafficGeometry(
      [
        { bucketStart: "2026-08-24T00:00:00Z", aToBBytes: 100, bToABytes: 50 },
        { bucketStart: "2026-08-24T00:00:10Z", aToBBytes: 20, bToABytes: 80 },
      ],
      10_000,
    );
    expect(
      geometry.points.every(
        (point) => point.aToBRate >= 0 && point.bToARate >= 0,
      ),
    ).toBe(true);
    expect(
      geometry.points.every((point) => point.aY <= 130 && point.bY >= 130),
    ).toBe(true);
  });
});

describe("path timeline", () => {
  it("starts an anchor at the selected window boundary", () => {
    const history = {
      edgeId: "a--b",
      source: { id: "a", label: "A" },
      target: { id: "b", label: "B" },
      from: "2026-08-24T00:00:00Z",
      to: "2026-08-24T01:00:00Z",
      bucketDurationMs: 30_000,
      traffic: [],
      pathAnchor: {
        observedAt: "2026-08-23T23:00:00Z",
        path: { kind: "direct" },
        observations: [],
      },
      pathEvents: [
        {
          observedAt: "2026-08-24T00:30:00Z",
          path: { kind: "derp", derpRegion: "hkg" },
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
