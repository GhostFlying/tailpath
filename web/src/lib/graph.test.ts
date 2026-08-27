import { describe, expect, it } from "vitest";
import type { Topology, TopologyEdge, TopologyNode } from "../api/types";
import {
  buildElements,
  edgeIsVisible,
  emptyTrafficReason,
  edgeIdealLength,
  edgeIdealLengthForWidth,
  maximumTrafficWidth,
  minimumTrafficWidth,
  minimumEdgeCenterDistance,
  trafficVisualCeiling,
  trafficWidth,
} from "./graph";

describe("buildElements", () => {
  it("hides nodes unrelated to a path filter", () => {
    const elements = buildElements(topology(), {
      pathFilter: "direct",
      showRecent: true,
      query: "",
    });
    const nodeIDs = elements
      .filter((element) => element.group === "nodes")
      .map((element) => element.data?.id);
    expect(nodeIDs).toEqual(["a", "b"]);
    expect(
      elements.filter((element) => element.group === "edges"),
    ).toHaveLength(1);
  });

  it("shows only one compact total rate and exposes directional flow classes", () => {
    const edge = buildElements(topology(), {
      pathFilter: "direct",
      showRecent: true,
      query: "",
    }).find((element) => element.group === "edges");
    expect(edge?.data?.label).toBe("15 KB/s");
    expect(edge?.data?.idealLength).toBeGreaterThanOrEqual(
      minimumEdgeCenterDistance,
    );
    expect(edge?.data?.trafficWidth).toBe(2.5);
    expect(String(edge?.classes)).toContain("flow-forward");
    expect(String(edge?.classes)).toContain("flow-reverse");
  });

  it("keeps path text out of peer relay rate labels", () => {
    const relay = buildElements(
      {
        ...topology(),
        edges: [
          {
            ...edge("relay", "a", "b", "peer_relay"),
            aToBBytesPerSecond: 320,
          },
        ],
      },
      { pathFilter: "all", showRecent: true, query: "" },
    ).filter((element) => element.group === "edges");
    expect(relay.map((element) => element.data?.label)).toEqual([
      "320 B/s",
      "",
    ]);
  });

  it("budgets edge length from label width with an arrow-clearance floor", () => {
    expect(edgeIdealLength("")).toBe(minimumEdgeCenterDistance);
    expect(edgeIdealLengthForWidth(80)).toBe(240);
    expect(edgeIdealLengthForWidth(20)).toBe(minimumEdgeCenterDistance);
  });

  it("keeps active edges and optionally removes recent edges and their nodes", () => {
    const fixture = topology();
    fixture.edges[1].state = "recent";
    const elements = buildElements(fixture, {
      pathFilter: "all",
      showRecent: false,
      query: "",
    });
    expect(
      elements
        .filter((element) => element.group === "nodes")
        .map((element) => element.data?.id),
    ).toEqual(["a", "b"]);
    expect(
      new Set(
        elements
          .filter((element) => element.group === "edges")
          .map((element) => element.data?.logicalEdgeId),
      ),
    ).toEqual(new Set(["direct"]));
  });

  it("renders recent edges without rates or flow arrows", () => {
    const fixture = topology();
    fixture.edges[0].state = "recent";
    const rendered = buildElements(fixture, {
      pathFilter: "direct",
      showRecent: true,
      query: "",
    }).find((element) => element.group === "edges");
    expect(rendered?.data?.label).toBe("");
    expect(rendered?.data?.trafficWidth).toBe(minimumTrafficWidth);
    expect(String(rendered?.classes)).not.toContain("flow-forward");
    expect(String(rendered?.classes)).not.toContain("flow-reverse");
  });

  it("adds a question-mark midpoint for unknown paths", () => {
    const fixture = topology();
    fixture.edges = [edge("unknown", "a", "b", "unknown")];
    const elements = buildElements(fixture, {
      pathFilter: "unknown",
      showRecent: true,
      query: "",
    });
    const marker = elements.find(
      (element) => element.data?.id === "unknown-marker:unknown",
    );
    expect(marker?.data?.label).toBe("?");
    expect(marker?.data?.logicalEdgeId).toBe("unknown");
    expect(
      elements.filter((element) => element.group === "edges"),
    ).toHaveLength(2);
  });

  it("keeps a shared relay active when any visible edge is active", () => {
    const fixture = topology();
    const recent = edge("recent-derp", "a", "b", "derp");
    recent.state = "recent";
    fixture.edges = [recent, edge("active-derp", "c", "d", "derp")];
    const relay = buildElements(fixture, {
      pathFilter: "derp",
      showRecent: true,
      query: "",
    }).find((element) => element.data?.id === "derp:hkg");
    expect(String(relay?.classes)).toContain("active");
    expect(String(relay?.classes)).not.toContain("recent");
  });

  it("marks runtime telemetry on nodes instead of reporter processes", () => {
    const elements = buildElements(topology(), {
      pathFilter: "all",
      showRecent: true,
      query: "",
    });
    expect(
      String(elements.find((item) => item.data?.id === "a")?.classes),
    ).toContain("runtime-telemetry");
    expect(
      String(elements.find((item) => item.data?.id === "b")?.classes),
    ).toContain("peer-only");
  });

  it("adds platform and independent telemetry and skew semantics", () => {
    const fixture = topology();
    fixture.nodes[0].os = "linux";
    fixture.nodes[0].clockSkewed = true;
    const rendered = buildElements(fixture, {
      pathFilter: "direct",
      showRecent: true,
      query: "",
    }).find((item) => item.data?.id === "a");
    expect(rendered?.data?.backgroundImages).toEqual([
      "/device-linux.svg",
      "/runtime-telemetry.svg",
      "/clock-skew.svg",
    ]);
    expect(String(rendered?.classes)).toContain("device-node");
    expect(String(rendered?.classes)).toContain("runtime-telemetry");
    expect(String(rendered?.classes)).toContain("clock-skewed");
  });

  it("uses path-specific peer relay anatomy without a platform glyph", () => {
    const fixture = topology();
    fixture.edges = [
      {
        ...edge("relay", "a", "b", "peer_relay"),
        path: { kind: "peer_relay", peerRelayStableNodeId: "c" },
      },
    ];
    const rendered = buildElements(fixture, {
      pathFilter: "peer_relay",
      showRecent: true,
      query: "",
    }).find((item) => item.data?.id === "c");
    expect(rendered?.data?.kind).toBe("peer-relay");
    expect(rendered?.data?.backgroundImages).toBeUndefined();
    expect(String(rendered?.classes)).toContain("relay-node peer-relay");
    expect(String(rendered?.classes)).not.toContain("device-node");
  });

  it.each([
    ["partial", "Unresolved client", "/identity-partial.svg"],
    ["anonymous", "Anonymous client", "/identity-anonymous.svg"],
    ["conflict", "Identity conflict", "/identity-conflict.svg"],
  ] as const)(
    "uses explicit %s relay-client anatomy instead of a platform",
    (identityStatus, label, asset) => {
      const fixture = topology();
      fixture.nodes[0].identityStatus = identityStatus;
      fixture.nodes[0].stableNodeId = "";
      fixture.nodes[0].hostname = "";
      const rendered = buildElements(fixture, {
        pathFilter: "direct",
        showRecent: true,
        query: "",
      }).find((item) => item.data?.id === "a");
      expect(rendered?.data?.label).toBe(label);
      expect(rendered?.data?.backgroundImages).toEqual([
        asset,
        "/runtime-telemetry.svg",
      ]);
      expect(String(rendered?.classes)).toContain(`identity-${identityStatus}`);
    },
  );

  it("does not render inventory-only nodes without a visible relationship", () => {
    const fixture = topology();
    fixture.edges = [];
    const elements = buildElements(fixture, {
      pathFilter: "all",
      showRecent: true,
      query: "",
    });
    expect(elements).toEqual([]);
  });

  it("persists only canonical topology nodes", () => {
    const fixture = topology();
    fixture.edges = [edge("unknown", "a", "b", "unknown")];
    const nodes = buildElements(fixture, {
      pathFilter: "all",
      showRecent: true,
      query: "",
    }).filter((element) => element.group === "nodes");
    expect(
      nodes
        .filter((node) => node.data?.persistable)
        .map((node) => node.data?.id),
    ).toEqual(["a", "b"]);
    expect(
      nodes.find((node) => node.data?.id === "unknown-marker:unknown")?.data
        ?.persistable,
    ).toBeUndefined();
  });
});

describe("trafficWidth", () => {
  it("uses a fixed, clamped logarithmic scale", () => {
    expect(trafficWidth(70)).toBe(minimumTrafficWidth);
    expect(trafficWidth(530)).toBe(minimumTrafficWidth);
    expect(trafficWidth(100 * 1024)).toBe(3);
    expect(trafficWidth(2 * 1024 * 1024)).toBe(4.25);
    expect(trafficWidth(10 * 1024 * 1024)).toBe(4.75);
    expect(trafficWidth(trafficVisualCeiling)).toBe(maximumTrafficWidth);
    expect(trafficWidth(trafficVisualCeiling * 10)).toBe(maximumTrafficWidth);
  });
});

describe("edgeIsVisible", () => {
  it("always keeps active edges and gates recent edges behind the option", () => {
    const active = edge("active", "a", "b", "direct");
    const recent = { ...active, state: "recent" as const };
    expect(edgeIsVisible(active, "all", false)).toBe(true);
    expect(edgeIsVisible(recent, "all", false)).toBe(false);
    expect(edgeIsVisible(recent, "direct", true)).toBe(true);
    expect(edgeIsVisible(active, "derp", true)).toBe(false);
  });

  it("hides system telemetry edges unless explicitly enabled", () => {
    const fixture = topology();
    fixture.edges[0].systemTelemetry = true;
    fixture.edges[1].path.kind = "direct";
    const hidden = buildElements(fixture, {
      pathFilter: "all",
      showRecent: true,
      showControlTraffic: false,
      query: "",
    });
    expect(hidden.filter((element) => element.group === "edges")).toHaveLength(
      1,
    );
    expect(
      hidden
        .filter((element) => element.group === "nodes")
        .map((element) => element.data?.id),
    ).toEqual(["c", "d"]);

    const shown = buildElements(fixture, {
      pathFilter: "all",
      showRecent: true,
      showControlTraffic: true,
      query: "",
    });
    expect(shown.filter((element) => element.group === "edges")).toHaveLength(
      2,
    );
  });
});

describe("emptyTrafficReason", () => {
  it("describes the currently selected activity window", () => {
    expect(emptyTrafficReason([], "all", false)).toBe("no-active");
    expect(emptyTrafficReason([], "all", true)).toBe("no-recent");
  });

  it("distinguishes hidden recent traffic from an unmatched path", () => {
    const recent = edge("recent", "a", "b", "direct");
    recent.state = "recent";
    expect(emptyTrafficReason([recent], "all", false)).toBe("no-active");
    expect(emptyTrafficReason([recent], "derp", true)).toBe("no-match");
    expect(emptyTrafficReason([recent], "direct", true)).toBeNull();
  });
});

function topology(): Topology {
  const direct = edge("direct", "a", "b", "direct");
  direct.aToBBytesPerSecond = 12_000;
  direct.bToABytesPerSecond = 3_000;
  return {
    generatedAt: "2026-08-23T00:00:00Z",
    nodes: [node("a"), node("b"), node("c"), node("d")],
    edges: [direct, edge("derp", "c", "d", "derp")],
    observers: [],
  };
}

function node(id: string): TopologyNode {
  return {
    id,
    stableNodeId: id,
    hostname: id.toUpperCase(),
    os: "linux",
    observable: id === "a",
    online: id === "a",
    lastEvidenceAt: "2026-08-23T00:00:00Z",
    clockSkewed: false,
  };
}

function edge(
  id: string,
  source: string,
  target: string,
  kind: "direct" | "derp" | "peer_relay" | "unknown",
): TopologyEdge {
  return {
    id,
    source,
    target,
    systemTelemetry: false,
    path: { kind, derpRegion: kind === "derp" ? "hkg" : undefined },
    state: "active",
    aToBBytesPerSecond: 1,
    bToABytesPerSecond: 0,
    lastActive: "2026-08-23T00:00:00Z",
    observations: [],
  };
}
