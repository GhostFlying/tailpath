import { expect, test } from "@playwright/test";

test("keeps unconfirmed sender routes unknown in Live and History", async ({
  page,
}, testInfo) => {
  const at = new Date(Date.now() - 2000).toISOString();
  const unknown = { kind: "unknown" };
  const nodes = [
    {
      id: "sender",
      stableNodeId: "sender-stable",
      hostname: "smallbox",
      observable: true,
      online: true,
      lastEvidenceAt: at,
      clockSkewed: false,
    },
    {
      id: "peer",
      stableNodeId: "peer-stable",
      hostname: "aws-sgp",
      observable: false,
      online: false,
      lastEvidenceAt: at,
      clockSkewed: false,
    },
  ];
  const observation = {
    observerId: "sender",
    path: unknown,
    collectedAt: at,
    receivedAt: at,
    clockSkewed: false,
  };
  const direction = {
    fromNodeId: "sender",
    toNodeId: "peer",
    primaryPath: unknown,
    evidence: "observed",
    observerId: "sender",
    collectedAt: at,
    receivedAt: at,
    clockSkewed: false,
  };
  const edge = {
    id: "sender--peer",
    source: "sender",
    target: "peer",
    path: unknown,
    state: "active",
    aToBBytesPerSecond: 148,
    bToABytesPerSecond: 0,
    lastActive: at,
    observations: [observation],
    directions: [direction],
    pathState: "stable",
  };
  await page.route("**/api/v1/topology", (route) =>
    route.fulfill({
      json: { generatedAt: at, nodes, edges: [edge], observers: [] },
    }),
  );
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toHaveAttribute("data-ready", "true");
  // Two endpoints plus the existing Unknown question-mark marker.
  await expect(graph).toHaveAttribute("data-node-count", "3");
  if (!testInfo.project.name.startsWith("mobile")) {
    await expect(
      page.getByRole("button", { name: "Unknown 1", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "DERP 0", exact: true }),
    ).toBeVisible();
  }
  const targets = JSON.parse(
    (await graph.getAttribute("data-edge-hit-targets")) ?? "[]",
  ) as Array<{ x: number; y: number }>;
  const box = await graph.boundingBox();
  const viewport = (await graph.getAttribute("data-viewport")) ?? "";
  const [, zoom, panX, panY] =
    viewport.match(/^(-?[\d.]+):(-?[\d.]+),(-?[\d.]+)$/) ?? [];
  expect(box).not.toBeNull();
  expect(targets.length).toBeGreaterThan(0);
  await page.mouse.click(
    box!.x + targets[0].x * Number(zoom) + Number(panX),
    box!.y + targets[0].y * Number(zoom) + Number(panY),
  );
  const inspector = page.getByLabel("Topology details");
  await expect(inspector).toContainText("Unknown");
  await expect(inspector).toContainText("The sender reported no usable path");
  await expect(inspector).not.toContainText("DERP sin");
  await page.screenshot({
    path: testInfo.outputPath("unconfirmed-live.png"),
    fullPage: true,
  });
  await testInfo.attach("Unconfirmed sender Live", {
    path: testInfo.outputPath("unconfirmed-live.png"),
    contentType: "image/png",
  });

  const source = {
    id: "sender",
    label: "smallbox",
    stableNodeId: "sender-stable",
  };
  const target = { id: "peer", label: "aws-sgp", stableNodeId: "peer-stable" };
  const event = {
    observedAt: at,
    path: unknown,
    pathState: "stable",
    pathCandidates: [],
    conflicts: [],
    observations: [observation],
    directions: [direction],
    directionsTracked: true,
  };
  const history = {
    edgeId: "sender--peer",
    source,
    target,
    relatedNodes: [source, target],
    from: new Date(Date.parse(at) - 60000).toISOString(),
    to: new Date(Date.parse(at) + 1000).toISOString(),
    bucketDurationMs: 10000,
    lastTrafficAt: at,
    traffic: [{ bucketStart: at, aToBBytes: 148, bToABytes: 0 }],
    pathEvents: [event],
    trafficTruncated: false,
    pathEventsTruncated: false,
  };
  await page.route("**/api/v1/history/nodes?**", (route) =>
    route.fulfill({ json: { nodes: [source, target] } }),
  );
  await page.route("**/api/v1/history/edges?**", (route) =>
    route.fulfill({
      json: {
        edges: [
          {
            edgeId: "sender--peer",
            source,
            target,
            lastTrafficAt: at,
            aToBBytes: 148,
            bToABytes: 0,
            paths: ["unknown"],
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/history/edges/sender--peer?**", (route) =>
    route.fulfill({ json: history }),
  );
  await page.route("**/api/v1/history/edges/sender--peer/paths?**", (route) =>
    route.fulfill({ json: { events: [event] } }),
  );
  await page.goto("/history/edges/sender--peer?window=1h");
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  const timeline = page.getByRole("list", { name: "Path timeline" });
  await expect(timeline).toContainText("Unknown");
  await expect(timeline).not.toContainText("DERP");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  expect(errors).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath("unconfirmed-history.png"),
    fullPage: true,
  });
  await testInfo.attach("Unconfirmed sender History", {
    path: testInfo.outputPath("unconfirmed-history.png"),
    contentType: "image/png",
  });
});
