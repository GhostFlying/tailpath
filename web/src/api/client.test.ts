import { afterEach, describe, expect, it, vi } from "vitest";
import { getDevices, getEdgeHistory, getEdgePathHistory } from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("getEdgeHistory", () => {
  it("normalizes nullable empty-window collections from older servers", async () => {
    stubHistoryResponse({
      ...historyResponse(),
      traffic: null,
      pathAnchor: {
        ...pathEvent(),
        conflicts: null,
        observations: null,
      },
      relatedNodes: null,
      pathEvents: null,
    });

    const history = await getEdgeHistory("node-a--node-b", undefined, "15m");

    expect(history.traffic).toEqual([]);
    expect(history.pathEvents).toEqual([]);
    expect(history.relatedNodes).toEqual([]);
    expect(history.pathAnchor?.conflicts).toEqual([]);
    expect(history.pathAnchor?.observations).toEqual([]);
  });

  it("normalizes nullable observations on retained path events", async () => {
    stubHistoryResponse({
      ...historyResponse(),
      traffic: [],
      pathEvents: [{ ...pathEvent(), observations: null }],
    });

    const history = await getEdgeHistory("node-a--node-b");

    expect(history.pathEvents).toHaveLength(1);
    expect(history.pathEvents[0].observations).toEqual([]);
  });
});

describe("getEdgePathHistory", () => {
  it("requests a stable cursor page and normalizes directional collections", async () => {
    const fetchMock = vi.fn(async () =>
      Response.json({
        anchor: { ...pathEvent(), directions: null },
        events: [
          {
            ...pathEvent(),
            observedAt: "2026-08-30T00:01:00Z",
            conflicts: null,
            observations: null,
            directions: null,
          },
        ],
        nextCursor: "next page",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const page = await getEdgePathHistory(
      "node-a--node-b",
      "24h",
      "cursor value",
      undefined,
      200,
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/history/edges/node-a--node-b/paths?window=24h&limit=200&cursor=cursor+value",
      expect.objectContaining({ headers: { Accept: "application/json" } }),
    );
    expect(page.anchor?.directions).toEqual([]);
    expect(page.events[0]).toMatchObject({
      conflicts: [],
      observations: [],
      directions: [],
    });
    expect(page.nextCursor).toBe("next page");
  });
});

describe("getDevices", () => {
  it("normalizes a legacy null directory collection", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({ sync: { status: "healthy" }, devices: null }),
      ),
    );

    await expect(getDevices()).resolves.toEqual({
      sync: { status: "healthy" },
      devices: [],
    });
  });
});

function stubHistoryResponse(body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
}

function historyResponse() {
  return {
    edgeId: "node-a--node-b",
    source: { id: "node-a", label: "Node A" },
    target: { id: "node-b", label: "Node B" },
    systemTelemetry: false,
    relatedNodes: [],
    from: "2026-08-30T00:00:00Z",
    to: "2026-08-30T00:15:00Z",
    bucketDurationMs: 10_000,
    traffic: [],
    pathEvents: [],
    trafficTruncated: false,
    pathEventsTruncated: false,
  };
}

function pathEvent() {
  return {
    observedAt: "2026-08-29T23:00:00Z",
    path: { kind: "direct" },
    conflicts: [],
    observations: [],
  };
}
