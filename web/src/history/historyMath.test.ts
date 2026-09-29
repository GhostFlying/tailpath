import { describe, expect, it } from "vitest";
import type { EdgeHistory } from "../api/types";
import {
  buildPathTimeline,
  buildDirectionalTimeline,
  compatibilityPathEvidenceKey,
  coalesceDirectionalTimeline,
  pathEvidenceKey,
  hasDirectionalHistory,
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
  it("keeps complete relay identity in evidence equality", () => {
    const relay = {
      kind: "peer_relay" as const,
      peerRelayStableNodeId: "relay-hz",
      peerRelayEndpoint: "203.0.113.8:41641",
      peerRelayResolution: "endpoint_match" as const,
    };

    expect(pathEvidenceKey({ ...relay, peerRelayVni: 4293 })).not.toBe(
      pathEvidenceKey({ ...relay, peerRelayVni: 8 }),
    );
    expect(compatibilityPathEvidenceKey({ ...relay, peerRelayVni: 4293 })).toBe(
      compatibilityPathEvidenceKey({ ...relay, peerRelayVni: 8 }),
    );
  });

  it("computes chronological bounds before returning newest first", () => {
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
      from: history.pathEvents[0].observedAt,
      to: history.to,
      anchored: false,
    });
    expect(timeline[1]).toMatchObject({
      from: history.from,
      to: history.pathEvents[0].observedAt,
      anchored: true,
    });
    expect(timeline[0].durationMs).toBe(30 * 60 * 1000);
  });

  it("restores both direction slots on one chronological axis", () => {
    const history = directionalHistory();
    const timeline = buildDirectionalTimeline(history);

    expect(hasDirectionalHistory(history)).toBe(true);
    expect(timeline).toHaveLength(2);
    expect(timeline.map((item) => item.from)).toEqual([
      history.from,
      history.pathEvents[0].observedAt,
    ]);
    expect(timeline[0].aToB?.primaryPath.kind).toBe("peer_relay");
    expect(timeline[0].aToB?.fallbackPath?.kind).toBe("derp");
    expect(timeline[0].bToA?.primaryPath.kind).toBe("direct");
    expect(timeline[1].aToB?.primaryPath.kind).toBe("direct");
    expect(timeline[1].bToA).toBeUndefined();
  });

  it("keeps the pre-event window unknown when no anchor was retained", () => {
    const history = directionalHistory();
    history.pathAnchor = undefined;
    const timeline = buildDirectionalTimeline(history);

    expect(timeline).toHaveLength(2);
    expect(timeline[0]).toMatchObject({
      from: history.from,
      to: history.pathEvents[0].observedAt,
      noEvidence: true,
      aToB: undefined,
      bToA: undefined,
    });
  });

  it("keeps an existing event identity when older pages are prepended", () => {
    const partial = directionalHistory();
    const selectedEvent = partial.pathEvents[0];
    const partialID = buildDirectionalTimeline(partial).find(
      (segment) => segment.event === selectedEvent,
    )?.id;
    const full = directionalHistory();
    full.pathEvents = [
      {
        ...selectedEvent,
        observedAt: "2026-08-24T00:15:00Z",
        path: { kind: "derp", derpRegion: "hkg" },
      },
      selectedEvent,
    ];
    const fullID = buildDirectionalTimeline(full).find(
      (segment) => segment.observedAt === selectedEvent.observedAt,
    )?.id;

    expect(fullID).toBe(partialID);
    expect(fullID?.length).toBeLessThan(100);
  });

  it("marks an empty event after directional evidence as withdrawn", () => {
    const history = directionalHistory();
    history.pathEvents.push({
      observedAt: "2026-08-24T00:45:00Z",
      path: { kind: "direct" },
      conflicts: [],
      observations: [],
      directions: [],
    });

    const withdrawal = buildDirectionalTimeline(history).at(-1);

    expect(withdrawal).toMatchObject({
      noEvidence: true,
      aToB: undefined,
      bToA: undefined,
    });
  });

  it("preserves an empty directional anchor as a withdrawn state", () => {
    const history = directionalHistory();
    history.pathAnchor = {
      ...history.pathAnchor!,
      path: { kind: "direct" },
      directions: [],
      directionsTracked: true,
    };
    history.pathEvents = [];

    expect(hasDirectionalHistory(history)).toBe(true);
    expect(buildDirectionalTimeline(history)).toEqual([
      expect.objectContaining({
        anchored: true,
        noEvidence: true,
        aToB: undefined,
        bToA: undefined,
      }),
    ]);
  });

  it("keeps an unprojectable migrated event in legacy history", () => {
    const history = directionalHistory();
    history.pathAnchor = {
      ...history.pathAnchor!,
      path: { kind: "direct" },
      directions: [],
      directionsTracked: false,
    };
    history.pathEvents = [];

    expect(hasDirectionalHistory(history)).toBe(false);
  });

  it("coalesces dense visual segments within a fixed render budget", () => {
    const base = buildDirectionalTimeline(directionalHistory())[1];
    const segments = Array.from({ length: 901 }, (_, index) => ({
      ...base,
      id: `event-${index}`,
      from: new Date(Date.parse(base.from) + index * 1_000).toISOString(),
      to: new Date(Date.parse(base.from) + (index + 1) * 1_000).toISOString(),
    }));
    const rendered = coalesceDirectionalTimeline(segments, 240);

    expect(rendered.length).toBeLessThanOrEqual(240);
    expect(rendered[0]).toMatchObject({
      from: segments[0].from,
      sourceSegmentId: segments[3].id,
      coalescedCount: 4,
      coalescedMixed: false,
    });
    expect(rendered.at(-1)?.to).toBe(segments.at(-1)?.to);
  });

  it("does not stretch a burst's latest state across an earlier long state", () => {
    const base = buildDirectionalTimeline(directionalHistory())[1];
    const start = Date.parse("2026-08-24T00:00:00Z");
    const segments = [
      {
        ...base,
        id: "direct-long",
        from: new Date(start).toISOString(),
        to: new Date(start + 23 * 60 * 60 * 1_000).toISOString(),
        aToB: {
          ...base.aToB!,
          primaryPath: { kind: "direct" as const },
        },
      },
      ...Array.from({ length: 120 }, (_, index) => ({
        ...base,
        id: `relay-burst-${index}`,
        from: new Date(
          start + 23 * 60 * 60 * 1_000 + index * 30_000,
        ).toISOString(),
        to: new Date(
          start + 23 * 60 * 60 * 1_000 + (index + 1) * 30_000,
        ).toISOString(),
        aToB: {
          ...base.aToB!,
          primaryPath: {
            kind: "peer_relay" as const,
            peerRelayVni: index % 2 ? 8 : 4293,
          },
        },
      })),
    ];

    const rendered = coalesceDirectionalTimeline(segments, 24);
    const directBins = rendered.filter(
      (segment) => segment.aToB?.primaryPath.kind === "direct",
    );

    expect(rendered).toHaveLength(24);
    expect(directBins).toHaveLength(23);
    expect(directBins.every((segment) => !segment.coalescedMixed)).toBe(true);
    expect(directBins.at(-1)?.to).toBe("2026-08-24T23:00:00.000Z");
    expect(rendered.at(-1)).toMatchObject({
      sourceSegmentId: "relay-burst-119",
      coalescedMixed: true,
    });
  });
});

function directionalHistory(): EdgeHistory {
  const state = (
    fromNodeId: string,
    toNodeId: string,
    primaryPath: EdgeHistory["pathEvents"][number]["path"],
    fallbackPath?: EdgeHistory["pathEvents"][number]["path"],
  ) => ({
    fromNodeId,
    toNodeId,
    primaryPath,
    fallbackPath,
    evidence: fallbackPath ? ("inferred" as const) : ("observed" as const),
    observerId: fromNodeId,
    collectedAt: "2026-08-24T00:00:00Z",
    receivedAt: "2026-08-24T00:00:01Z",
    clockSkewed: false,
  });
  return {
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
      observedAt: "2026-08-23T23:59:00Z",
      path: { kind: "peer_relay" },
      conflicts: [],
      observations: [],
      directions: [
        state(
          "a",
          "b",
          { kind: "peer_relay", peerRelayVni: 4293 },
          { kind: "derp", derpRegion: "hkg" },
        ),
        state("b", "a", { kind: "direct" }),
      ],
    },
    pathEvents: [
      {
        observedAt: "2026-08-24T00:30:00Z",
        path: { kind: "direct" },
        conflicts: [],
        observations: [],
        directions: [state("a", "b", { kind: "direct" })],
      },
    ],
    trafficTruncated: false,
    pathEventsTruncated: false,
  };
}
