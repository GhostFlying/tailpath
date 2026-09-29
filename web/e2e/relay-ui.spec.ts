import { expect, test, type Locator, type Page } from "@playwright/test";

const observedAt = new Date(Date.now() - 2_000).toISOString();
const historyFrom = new Date(
  Date.parse(observedAt) - 60 * 60 * 1_000,
).toISOString();
const historyTo = new Date(Date.parse(observedAt) + 1_000).toISOString();
const relayPath = {
  kind: "peer_relay",
  peerRelayStableNodeId: "relay-stable",
  peerRelayEndpoint: "203.0.113.10:41642",
  peerRelayResolution: "endpoint_match",
  peerRelayVni: 7,
} as const;
const pendingRelayPath = {
  kind: "peer_relay",
  peerRelayEndpoint: "198.51.100.24:45321",
  peerRelayVni: 19,
} as const;
const relaySession = {
  sessionId: "session-7",
  vni: 7,
  sourceIdentityStatus: "partial",
  targetIdentityStatus: "anonymous",
} as const;

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/topology", (route) =>
    route.fulfill({ json: relayTopology() }),
  );
  await page.route("**/api/v1/history/nodes?**", (route) =>
    route.fulfill({
      json: {
        nodes: [
          historyNode("client-a", "Unresolved client", "partial"),
          historyNode("client-b", "Anonymous client", "anonymous"),
        ],
      },
    }),
  );
  await page.route("**/api/v1/history/edges?**", (route) =>
    route.fulfill({
      json: {
        edges: [
          {
            edgeId: "client-a--client-b",
            source: historyNode("client-a", "Unresolved client", "partial"),
            target: historyNode("client-b", "Anonymous client", "anonymous"),
            lastTrafficAt: observedAt,
            aToBBytes: 1200,
            bToABytes: 400,
            paths: ["peer_relay"],
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/history/edges/client-a--client-b?**", (route) =>
    route.fulfill({ json: relayHistory() }),
  );
});

test("presents scoped relay clients and live provenance", async ({
  page,
}, testInfo) => {
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });
  await page.goto("/");
  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toHaveAttribute("data-ready", "true");
  await expect(graph).toHaveAttribute("data-edge-count", "1");
  await expect(graph).toHaveAttribute("data-node-count", "3");
  await expect(graph).toHaveAttribute("data-relay-platform-icon-count", "1");
  await page.screenshot({
    path: testInfo.outputPath(
      `relay-platform-icon-${testInfo.project.name}.png`,
    ),
    fullPage: true,
  });
  await clickGraphElement(page, graph, "client-a");
  const inspector = page.getByLabel("Topology details");
  await expect(inspector).toContainText("Unresolved client");
  await expect(inspector.getByLabel("Partial identity")).toBeVisible();
  await inspector.getByLabel("Close details").click();

  await clickGraphSegment(page, graph, "client-a", "relay-node");
  await expect(inspector).toContainText("Relay Node");
  await expect(inspector).toContainText("Relay VNI");
  await expect(inspector).toContainText("session-7");
  await expect(inspector.getByLabel("Partial identity").first()).toBeVisible();
  await expect(inspector.getByLabel("Anonymous relay client")).toBeVisible();
  await expect(inspector).toContainText("203.0.113.10:41642");
  expect(consoleErrors).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath(`relay-live-${testInfo.project.name}.png`),
    fullPage: true,
  });
});

test("restores durable relay history provenance", async ({
  page,
}, testInfo) => {
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });
  await page.goto("/history/edges/client-a--client-b?window=1h");
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  await expect(page.getByLabel("Endpoint identities")).toContainText("Partial");
  const timeline = page.getByRole("list", { name: "Path timeline" });
  await expect(timeline).toContainText("Peer Relay");
  await expect(
    timeline.getByRole("listitem").filter({ hasText: "Peer Relay" }),
  ).toHaveCount(2);
  await expect(page.getByText("Newest first", { exact: true })).toBeVisible();
  await expect(timeline.getByRole("listitem").first()).toContainText(
    "Relay Hangzhou",
  );
  await timeline.locator(".selected").click();
  const provenance = page.getByRole("table", { name: "Observed by" });
  await expect(provenance).toContainText("Relay: Relay Hangzhou");
  await expect(provenance).toContainText("VNI 7");
  await expect(provenance).toContainText("session-7");
  await expect(provenance).toContainText("Supports selected path");
  await expect(provenance).toContainText("Anonymous");
  const candidates = page.getByLabel("Historical path candidates");
  await expect(candidates).toContainText("203.0.113.10:41642");
  await expect(candidates).toContainText("Matched by endpoint");
  expect(consoleErrors).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath(`relay-history-${testInfo.project.name}.png`),
    fullPage: true,
  });
});

test("renders all fresh relay candidates while switching", async ({
  page,
}, testInfo) => {
  await page.unroute("**/api/v1/topology");
  await page.route("**/api/v1/topology", (route) =>
    route.fulfill({ json: switchingTopology() }),
  );
  await page.unroute("**/api/v1/history/edges/client-a--client-b?**");
  await page.route("**/api/v1/history/edges/client-a--client-b?**", (route) =>
    route.fulfill({ json: switchingHistory() }),
  );

  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });
  await page.goto("/");
  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toHaveAttribute("data-ready", "true");
  await expect(graph).toHaveAttribute("data-edge-count", "1");
  await expect(graph).toHaveAttribute("data-node-count", "4");

  await clickGraphSegment(page, graph, "client-a", "relay-node");
  const inspector = page.getByLabel("Topology details");
  await expect(inspector).toContainText("Switching");
  await expect(inspector).toContainText("Path candidates");
  await expect(inspector).toContainText("2 fresh");
  await expect(inspector).toContainText("aliyun-hangzhou-relay");
  await expect(inspector).toContainText("Matched by endpoints");
  await expect(inspector).toContainText("Identity pending");
  await expect(inspector).toContainText("203.0.113.10:41642");

  const pending = inspector
    .locator(".relay-candidate")
    .filter({ hasText: "Identity pending" });
  await pending.click();
  await expect(pending).toHaveAttribute("aria-expanded", "true");
  await expect(pending).toContainText("198.51.100.24:45321");
  await expect(inspector).toContainText(
    "Traffic belongs to this relationship and is not duplicated",
  );
  await expect(inspector).toContainText("Why Switching?");
  expect(consoleErrors).toEqual([]);

  await page.screenshot({
    path: testInfo.outputPath(
      `peer-relay-switching-${testInfo.project.name}.png`,
    ),
    fullPage: true,
  });

  await page.goto("/history/edges/client-a--client-b?window=1h");
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  if (testInfo.project.name.startsWith("mobile")) {
    await page
      .getByRole("list", { name: "Path timeline" })
      .getByRole("listitem")
      .first()
      .click();
  }
  const historicalCandidates = page.getByLabel("Historical path candidates");
  await expect(historicalCandidates).toContainText("Switching");
  await expect(historicalCandidates).toContainText("aliyun-hangzhou-relay");
  await expect(historicalCandidates).toContainText("Unresolved relay");
  await expect(historicalCandidates).toContainText("VNI 19");
  await expect(historicalCandidates).toContainText("203.0.113.10:41642");
  await expect(historicalCandidates).toContainText("198.51.100.24:45321");
  await expect(historicalCandidates).toContainText("Identity pending");
  await page.screenshot({
    path: testInfo.outputPath(
      `peer-relay-switching-history-${testInfo.project.name}.png`,
    ),
    fullPage: true,
  });
});

test("expands asymmetric live paths and keeps fallback traffic single-counted", async ({
  page,
}, testInfo) => {
  await page.unroute("**/api/v1/topology");
  await page.route("**/api/v1/topology", (route) =>
    route.fulfill({ json: directionalTopology() }),
  );
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });

  await page.goto("/");
  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toHaveAttribute("data-ready", "true");
  await expect(graph).toHaveAttribute("data-edge-count", "1");
  await expect(graph).toHaveAttribute("data-node-count", "4");
  await clickGraphSegment(page, graph, "client-a", "relay-node");

  const inspector = page.getByLabel("Topology details");
  await expect(inspector).toContainText("Asymmetric paths");
  await expect(inspector).toContainText("r4se-istoreos");
  await expect(inspector).toContainText("smallbox");
  await expect(inspector).toContainText("Peer Relay");
  await expect(inspector).toContainText("DERP fallback");
  await expect(inspector).toContainText("Inferred");
  await expect(inspector).toContainText("Direct");
  await expect(inspector).toContainText("Observed");
  await expect(inspector).toContainText("VNI 4293");
  await expect(inspector).toContainText("Resolution: endpoint match");
  await expect(inspector).toContainText("Endpoint path evidence");
  await expect(inspector).toContainText("Relay identity evidence");
  await expect(inspector.locator(".history-section")).toContainText(
    "Recent paths",
  );
  await expect(inspector).not.toContainText("Switching");
  await expect(inspector.locator(".direction-path-card")).toHaveCount(2);
  await expect(inspector.locator(".direction-fallback")).toHaveCount(1);
  await expect(inspector.getByText("16.2 MB/s", { exact: true })).toHaveCount(
    1,
  );

  const overflow = await page.evaluate(() => ({
    page: document.documentElement.scrollWidth - window.innerWidth,
    inspector: (() => {
      const element = document.querySelector<HTMLElement>(".inspector");
      return element ? element.scrollWidth - element.clientWidth : 0;
    })(),
  }));
  expect(overflow.page).toBeLessThanOrEqual(1);
  expect(overflow.inspector).toBeLessThanOrEqual(1);
  expect(consoleErrors).toEqual([]);

  await page.screenshot({
    path: testInfo.outputPath(`directional-live-${testInfo.project.name}.png`),
    fullPage: true,
  });
});

test("keeps a missing reverse direction explicitly unknown", async ({
  page,
}) => {
  const fixture = directionalTopology();
  fixture.edges[0].directions = fixture.edges[0].directions.slice(0, 1);
  await page.unroute("**/api/v1/topology");
  await page.route("**/api/v1/topology", (route) =>
    route.fulfill({ json: fixture }),
  );

  await page.goto("/");
  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toHaveAttribute("data-ready", "true");
  await clickGraphSegment(page, graph, "client-a", "relay-node");

  const inspector = page.getByLabel("Topology details");
  await expect(inspector).toContainText("Partial path evidence");
  await expect(inspector).toContainText("reverse remains unknown");
  await expect(inspector).toContainText("No fresh observation");
  await expect(inspector).not.toContainText("Same path both directions");
});

test("keeps a dense switching timeline readable", async ({
  page,
}, testInfo) => {
  await page.unroute("**/api/v1/history/edges/client-a--client-b?**");
  await page.route("**/api/v1/history/edges/client-a--client-b?**", (route) =>
    route.fulfill({ json: denseSwitchingHistory() }),
  );

  await page.goto("/history/edges/client-a--client-b?window=1h");
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  const timeline = page.getByRole("list", { name: "Path timeline" });
  const events = timeline.getByRole("listitem");
  await expect(events).toHaveCount(12);
  await expect(timeline.locator(".timeline-state")).toHaveCount(12);
  await expect(timeline.locator(".timeline-observers")).toHaveCount(0);
  await expect(events.first()).toHaveAccessibleName(/2 observers/);
  await expect(timeline).toContainText("aliyun-hangzhou-relay");
  await expect(timeline).toContainText("Unresolved relay");

  const geometryFailures = await events.evaluateAll((buttons) =>
    buttons.flatMap((button, index) => {
      const compactStrip = window.innerWidth > 600;
      const buttonRect = button.getBoundingClientRect();
      const label = button.querySelector<HTMLElement>(".timeline-copy strong");
      const duration = button.querySelector<HTMLElement>(
        ".timeline-copy small",
      );
      const state = button.querySelector<HTMLElement>(".timeline-state");
      if (!label || !duration || !state) return [`${index}: missing content`];
      const labelRect = label.getBoundingClientRect();
      const durationRect = duration.getBoundingClientRect();
      const stateRect = state.getBoundingClientRect();
      const withinButton = [labelRect, durationRect, stateRect].every(
        (rect) =>
          rect.left >= buttonRect.left - 1 &&
          rect.right <= buttonRect.right + 1 &&
          rect.top >= buttonRect.top - 1 &&
          rect.bottom <= buttonRect.bottom + 1,
      );
      const rowsSeparated =
        labelRect.bottom <= Math.min(durationRect.top, stateRect.top) + 1;
      const metadataSeparated = durationRect.right <= stateRect.left + 1;
      const oneLine =
        !compactStrip || getComputedStyle(label).whiteSpace === "nowrap";
      return withinButton && rowsSeparated && metadataSeparated && oneLine
        ? []
        : [
            `${index}: within=${withinButton} rows=${rowsSeparated} metadata=${metadataSeparated} oneLine=${oneLine}`,
          ];
    }),
  );
  expect(geometryFailures).toEqual([]);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);

  await page.screenshot({
    path: testInfo.outputPath(
      `dense-switching-timeline-${testInfo.project.name}.png`,
    ),
    fullPage: true,
  });
});

test("preserves the relay neighborhood when an anonymous client resolves", async ({
  page,
}) => {
  await page.unroute("**/api/v1/topology");
  let resolved = false;
  let invalidateTopology: (() => void) | undefined;
  const invalidation = new Promise<void>((resolve) => {
    invalidateTopology = resolve;
  });
  await page.route("**/api/v1/topology", (route) =>
    route.fulfill({ json: relayTopology(resolved) }),
  );
  await page.route("**/api/v1/events", async (route) => {
    await invalidation;
    await route.fulfill({
      contentType: "text/event-stream",
      body: 'event: topology\ndata: {"generatedAt":"2026-08-26T04:00:01Z"}\n\n',
    });
  });
  await page.goto("/");
  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toHaveAttribute("data-ready", "true");
  await expectSparseGraphFocused(graph);
  const initial = parseGraphPositions(
    (await graph.getAttribute("data-layout-positions")) ?? "",
  );
  const viewport = await graph.getAttribute("data-viewport");

  resolved = true;
  invalidateTopology?.();
  await expect
    .poll(async () =>
      parseGraphPositions(
        (await graph.getAttribute("data-layout-positions")) ?? "",
      ).has("resolved-b"),
    )
    .toBe(true);
  const updated = parseGraphPositions(
    (await graph.getAttribute("data-layout-positions")) ?? "",
  );
  expect(updated.has("client-b")).toBe(false);
  expect(updated.get("client-a")).toBe(initial.get("client-a"));
  expect(updated.get("relay-node")).toBe(initial.get("relay-node"));
  await expect(graph).toHaveAttribute("data-viewport", viewport ?? "");
});

function relayTopology(resolved = false) {
  const targetID = resolved ? "resolved-b" : "client-b";
  const targetStatus = resolved ? "resolved" : "anonymous";
  return {
    generatedAt: observedAt,
    nodes: [
      topologyNode("client-a", "", "", "partial"),
      topologyNode(
        targetID,
        resolved ? "node-b" : "",
        resolved ? "Resolved B" : "",
        targetStatus,
      ),
      topologyNode("relay-node", "relay-stable", "Relay Node", "resolved"),
    ],
    edges: [
      {
        id: `client-a--${targetID}`,
        source: "client-a",
        target: targetID,
        path: relayPath,
        pathState: "stable",
        pathCandidates: [
          { path: relayPath, lastObservedAt: observedAt, observerCount: 1 },
        ],
        state: "active",
        aToBBytesPerSecond: 600,
        bToABytesPerSecond: 200,
        lastActive: observedAt,
        observations: [
          {
            observerId: "relay-node",
            path: relayPath,
            collectedAt: observedAt,
            receivedAt: observedAt,
            clockSkewed: false,
            relaySession: {
              ...relaySession,
              targetIdentityStatus: targetStatus,
            },
          },
        ],
      },
    ],
    observers: [],
  };
}

function switchingTopology() {
  return {
    generatedAt: observedAt,
    nodes: [
      topologyNode("client-a", "client-a-stable", "r4se-istoreos", "resolved"),
      topologyNode("client-b", "client-b-stable", "smallbox", "resolved"),
      topologyNode(
        "relay-node",
        "relay-stable",
        "aliyun-hangzhou-relay",
        "resolved",
      ),
    ],
    edges: [
      {
        id: "client-a--client-b",
        source: "client-a",
        target: "client-b",
        path: relayPath,
        pathState: "switching",
        pathCandidates: [
          { path: relayPath, lastObservedAt: observedAt, observerCount: 2 },
          {
            path: pendingRelayPath,
            lastObservedAt: observedAt,
            observerCount: 1,
          },
        ],
        state: "active",
        aToBBytesPerSecond: 16_200_000,
        bToABytesPerSecond: 1_140_000,
        lastActive: observedAt,
        observations: [
          {
            observerId: "client-a",
            path: relayPath,
            collectedAt: observedAt,
            receivedAt: observedAt,
            clockSkewed: false,
          },
          {
            observerId: "client-b",
            path: pendingRelayPath,
            collectedAt: observedAt,
            receivedAt: observedAt,
            clockSkewed: false,
          },
        ],
        conflicts: [pendingRelayPath],
      },
    ],
    observers: [],
  };
}

function directionalTopology() {
  const fallback = { kind: "derp", derpRegion: "hkg" } as const;
  const direct = {
    kind: "direct",
    directEndpoint: "198.51.100.51:41641",
  } as const;
  return {
    generatedAt: observedAt,
    nodes: [
      topologyNode("client-a", "client-a-stable", "r4se-istoreos", "resolved"),
      topologyNode("client-b", "client-b-stable", "smallbox", "resolved"),
      topologyNode(
        "relay-node",
        "relay-stable",
        "aliyun-hangzhou-relay",
        "resolved",
      ),
    ],
    edges: [
      {
        id: "client-a--client-b",
        source: "client-a",
        target: "client-b",
        path: relayPath,
        pathState: "stable",
        directions: [
          {
            fromNodeId: "client-a",
            toNodeId: "client-b",
            primaryPath: { ...relayPath, peerRelayVni: 4293 },
            fallbackPath: fallback,
            evidence: "inferred",
            inferenceRule: "tailscale-status-fallback-v1",
            observerId: "client-a",
            collectedAt: observedAt,
            receivedAt: observedAt,
            clockSkewed: false,
          },
          {
            fromNodeId: "client-b",
            toNodeId: "client-a",
            primaryPath: direct,
            evidence: "observed",
            observerId: "client-b",
            collectedAt: observedAt,
            receivedAt: observedAt,
            clockSkewed: false,
          },
        ],
        state: "active",
        aToBBytesPerSecond: 16_200_000,
        bToABytesPerSecond: 1_140_000,
        lastActive: observedAt,
        observations: [
          {
            observerId: "client-a",
            path: { ...relayPath, peerRelayVni: 4293 },
            fallbackPath: fallback,
            pathEvidence: "inferred",
            pathInferenceRule: "tailscale-status-fallback-v1",
            collectedAt: observedAt,
            receivedAt: observedAt,
            clockSkewed: false,
          },
          {
            observerId: "client-b",
            path: direct,
            pathEvidence: "observed",
            collectedAt: observedAt,
            receivedAt: observedAt,
            clockSkewed: false,
          },
          {
            observerId: "relay-node",
            path: relayPath,
            collectedAt: observedAt,
            receivedAt: observedAt,
            clockSkewed: false,
            relaySession,
          },
        ],
      },
    ],
    observers: [],
  };
}

function topologyNode(
  id: string,
  stableNodeId: string,
  hostname: string,
  identityStatus: string,
) {
  return {
    id,
    stableNodeId,
    hostname,
    observable: id === "relay-node",
    online: id === "relay-node",
    os: identityStatus === "resolved" ? "linux" : undefined,
    lastEvidenceAt: observedAt,
    clockSkewed: false,
    identityStatus,
  };
}

function historyNode(id: string, label: string, identityStatus: string) {
  return { id, label, identityStatus };
}

function parseGraphPositions(value: string) {
  return new Map(
    value
      .split("|")
      .filter(Boolean)
      .map((entry) => {
        const separator = entry.lastIndexOf(":");
        return [entry.slice(0, separator), entry.slice(separator + 1)];
      }),
  );
}

async function expectSparseGraphFocused(graph: Locator) {
  await expect
    .poll(async () => {
      const positions = [
        ...parseGraphPositions(
          (await graph.getAttribute("data-layout-positions")) ?? "",
        ).values(),
      ].map((position) => {
        const [x, y] = position.split(",").map(Number);
        return { x, y };
      });
      const viewport = (await graph.getAttribute("data-viewport")) ?? "";
      const match = viewport.match(/^([\d.]+):(-?[\d.]+),(-?[\d.]+)$/);
      const box = await graph.boundingBox();
      if (positions.length === 0 || !match || !box) return "pending";
      const center = positions.reduce(
        (sum, position) => ({
          x: sum.x + position.x / positions.length,
          y: sum.y + position.y / positions.length,
        }),
        { x: 0, y: 0 },
      );
      const zoom = Number(match[1]);
      const xOffset = Math.abs(
        center.x * zoom + Number(match[2]) - box.width / 2,
      );
      const yOffset = Math.abs(
        center.y * zoom + Number(match[3]) - box.height / 2,
      );
      return zoom <= 1.25 && xOffset < 40 && yOffset < 70
        ? "focused"
        : `zoom=${zoom.toFixed(2)} x=${xOffset.toFixed(2)} y=${yOffset.toFixed(2)}`;
    })
    .toBe("focused");
}

function relayHistory() {
  const event = {
    observedAt,
    path: relayPath,
    pathState: "stable",
    pathCandidates: [
      { path: relayPath, lastObservedAt: observedAt, observerCount: 1 },
    ],
    conflicts: [],
    observations: [
      {
        observerId: "relay-node",
        path: relayPath,
        collectedAt: observedAt,
        receivedAt: observedAt,
        clockSkewed: false,
        relaySession,
      },
    ],
  };
  return {
    edgeId: "client-a--client-b",
    source: historyNode("client-a", "Unresolved client", "partial"),
    target: historyNode("client-b", "Anonymous client", "anonymous"),
    systemTelemetry: false,
    relatedNodes: [
      historyNode("client-a", "Unresolved client", "partial"),
      historyNode("client-b", "Anonymous client", "anonymous"),
      {
        id: "relay-node",
        stableNodeId: "relay-stable",
        label: "Relay Hangzhou",
        identityStatus: "resolved",
      },
    ],
    from: historyFrom,
    to: historyTo,
    bucketDurationMs: 30000,
    lastTrafficAt: observedAt,
    traffic: [{ bucketStart: observedAt, aToBBytes: 1200, bToABytes: 400 }],
    pathAnchor: event,
    pathEvents: [event],
    trafficTruncated: false,
    pathEventsTruncated: false,
  };
}

function switchingHistory() {
  const history = relayHistory();
  const event = {
    observedAt,
    path: relayPath,
    pathState: "switching",
    pathCandidates: [
      { path: relayPath, lastObservedAt: observedAt, observerCount: 2 },
      {
        path: pendingRelayPath,
        lastObservedAt: observedAt,
        observerCount: 1,
      },
    ],
    conflicts: [pendingRelayPath],
    observations: [
      {
        observerId: "client-a",
        path: relayPath,
        collectedAt: observedAt,
        receivedAt: observedAt,
        clockSkewed: false,
      },
      {
        observerId: "client-b",
        path: pendingRelayPath,
        collectedAt: observedAt,
        receivedAt: observedAt,
        clockSkewed: false,
      },
    ],
  } as const;
  return {
    ...history,
    source: historyNode("client-a", "r4se-istoreos", "resolved"),
    target: historyNode("client-b", "smallbox", "resolved"),
    relatedNodes: [
      historyNode("client-a", "r4se-istoreos", "resolved"),
      historyNode("client-b", "smallbox", "resolved"),
      {
        id: "relay-node",
        stableNodeId: "relay-stable",
        label: "aliyun-hangzhou-relay",
        identityStatus: "resolved",
      },
    ],
    pathAnchor: undefined,
    pathEvents: [event],
  };
}

function denseSwitchingHistory() {
  const history = switchingHistory();
  const seed = history.pathEvents[0];
  const end = Date.parse(observedAt);
  const start = end - 55_000;
  const pathEvents = Array.from({ length: 12 }, (_, index) => {
    const timestamp = new Date(start + index * 5_000).toISOString();
    return {
      ...seed,
      observedAt: timestamp,
      path: index % 2 === 0 ? relayPath : pendingRelayPath,
      pathCandidates: seed.pathCandidates.map((candidate) => ({
        ...candidate,
        lastObservedAt: timestamp,
      })),
      observations: seed.observations.map((observation) => ({
        ...observation,
        collectedAt: timestamp,
        receivedAt: timestamp,
      })),
    };
  });
  return {
    ...history,
    from: new Date(start).toISOString(),
    to: new Date(end + 5_000).toISOString(),
    pathEvents,
  };
}

async function clickGraphElement(page: Page, graph: Locator, id: string) {
  const point = await graphPoint(graph, id);
  await page.mouse.click(point.x, point.y);
}

async function clickGraphSegment(
  page: Page,
  graph: Locator,
  sourceID: string,
  targetID: string,
) {
  const box = await graph.boundingBox();
  if (!box) throw new Error("graph has no bounds");
  const targets = JSON.parse(
    (await graph.getAttribute("data-edge-hit-targets")) ?? "[]",
  ) as Array<{ source: string; target: string; x: number; y: number }>;
  const target = targets.find(
    (candidate) =>
      (candidate.source === sourceID && candidate.target === targetID) ||
      (candidate.source === targetID && candidate.target === sourceID),
  );
  const viewport = (await graph.getAttribute("data-viewport")) ?? "";
  const [, rawZoom, rawPanX, rawPanY] =
    viewport.match(/^(-?[\d.]+):(-?[\d.]+),(-?[\d.]+)$/) ?? [];
  if (!target || !rawZoom || !rawPanX || !rawPanY) {
    throw new Error(`missing edge hit target: ${sourceID} / ${targetID}`);
  }
  await page.mouse.click(
    box.x + target.x * Number(rawZoom) + Number(rawPanX),
    box.y + target.y * Number(rawZoom) + Number(rawPanY),
  );
}

async function graphPoint(graph: Locator, id: string) {
  const box = await graph.boundingBox();
  if (!box) throw new Error("graph has no bounds");
  const positions = (await graph.getAttribute("data-layout-positions")) ?? "";
  const entry = positions
    .split("|")
    .find((candidate) => candidate.startsWith(`${id}:`));
  if (!entry) throw new Error(`graph has no position for ${id}: ${positions}`);
  const [, rawX, rawY] = entry.match(/:(-?[\d.]+),(-?[\d.]+)$/) ?? [];
  const viewport = (await graph.getAttribute("data-viewport")) ?? "";
  const [, rawZoom, rawPanX, rawPanY] =
    viewport.match(/^(-?[\d.]+):(-?[\d.]+),(-?[\d.]+)$/) ?? [];
  if (!rawX || !rawY || !rawZoom || !rawPanX || !rawPanY) {
    throw new Error(`invalid graph diagnostics: ${entry} / ${viewport}`);
  }
  return {
    x: box.x + Number(rawX) * Number(rawZoom) + Number(rawPanX),
    y: box.y + Number(rawY) * Number(rawZoom) + Number(rawPanY),
  };
}
