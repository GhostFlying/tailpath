import { expect, test, type Locator, type Page } from "@playwright/test";

const observedAt = "2026-08-26T04:00:00Z";
const relayPath = {
  kind: "peer_relay",
  peerRelayStableNodeId: "relay-stable",
  peerRelayVni: 7,
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
  await expect(inspector).not.toContainText("192.0.2.");
  expect(consoleErrors).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath(`relay-live-${testInfo.project.name}.png`),
    fullPage: true,
  });
});

test("presents sanitized relay history provenance", async ({
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
  await timeline.locator(".selected").click();
  const provenance = page.getByRole("table", { name: "Observed by" });
  await expect(provenance).toContainText("Relay relay-stable");
  await expect(provenance).toContainText("VNI 7");
  await expect(provenance).toContainText("session-7");
  await expect(provenance).toContainText("Anonymous");
  await expect(provenance).not.toContainText("192.0.2.");
  expect(consoleErrors).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath(`relay-history-${testInfo.project.name}.png`),
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
    os: id === "relay-node" ? "linux" : undefined,
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
    from: "2026-08-26T03:00:00Z",
    to: "2026-08-26T04:00:01Z",
    bucketDurationMs: 30000,
    lastTrafficAt: observedAt,
    traffic: [{ bucketStart: observedAt, aToBBytes: 1200, bToABytes: 400 }],
    pathAnchor: event,
    pathEvents: [event],
    trafficTruncated: false,
    pathEventsTruncated: false,
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
  const source = await graphPoint(graph, sourceID);
  const target = await graphPoint(graph, targetID);
  await page.mouse.click((source.x + target.x) / 2, (source.y + target.y) / 2);
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
