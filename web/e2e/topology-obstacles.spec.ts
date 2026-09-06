import { expect, test, type Page } from "@playwright/test";

const observedAt = "2026-09-06T12:00:00Z";
const cachedPositions = new Map([
  ["direct-a", { x: 100, y: 160 }],
  ["direct-blocker", { x: 320, y: 160 }],
  ["direct-b", { x: 540, y: 160 }],
  ["derp-a", { x: 100, y: 420 }],
  ["derp-blocker", { x: 320, y: 420 }],
  ["derp-b", { x: 540, y: 420 }],
]);

test("routes collinear traffic and moves an occupied DERP marker", async ({
  page,
}, testInfo) => {
  if (testInfo.project.name.startsWith("mobile")) {
    await page.setViewportSize({ width: 390, height: 844 });
  }
  const browserErrors = await installObstacleFixture(page);
  await page.goto("/");

  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toHaveAttribute("data-ready", "true");
  await expect(graph).toHaveAttribute("data-routed-edges", /direct-long:main/);
  await expect(graph).toHaveAttribute("data-edge-crossing-count", "0");
  await expect
    .poll(async () =>
      virtualPosition(await graph.getAttribute("data-virtual-positions")),
    )
    .not.toEqual(cachedPositions.get("derp-blocker"));
  const initialVirtualPosition = virtualPosition(
    await graph.getAttribute("data-virtual-positions"),
  );
  expect(
    distance(initialVirtualPosition, cachedPositions.get("derp-blocker")),
  ).toBeGreaterThan(70);
  expect(
    canonicalPositions(await graph.getAttribute("data-layout-positions")),
  ).toEqual(cachedPositions);

  await page.reload();
  await expect(graph).toHaveAttribute("data-ready", "true");
  await expect(graph).toHaveAttribute("data-edge-crossing-count", "0");
  expect(
    virtualPosition(await graph.getAttribute("data-virtual-positions")),
  ).toEqual(initialVirtualPosition);
  expect(
    canonicalPositions(await graph.getAttribute("data-layout-positions")),
  ).toEqual(cachedPositions);
  expect(browserErrors).toEqual([]);

  await page.screenshot({
    path: testInfo.outputPath(
      `tailpath-obstacle-routing-${testInfo.project.name}.png`,
    ),
    fullPage: true,
  });
});

test("keeps obstacle routing readable at 320px", async ({ page }, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("mobile"));
  await page.setViewportSize({ width: 320, height: 780 });
  const browserErrors = await installObstacleFixture(page);
  await page.goto("/");

  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toHaveAttribute("data-ready", "true");
  await expect(graph).toHaveAttribute("data-routed-edges", /direct-long:main/);
  await expect(graph).toHaveAttribute("data-edge-crossing-count", "0");
  expect(browserErrors).toEqual([]);

  await page.screenshot({
    path: testInfo.outputPath("tailpath-obstacle-routing-320px.png"),
    fullPage: true,
  });
});

async function installObstacleFixture(page: Page) {
  const browserErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") browserErrors.push(message.text());
  });
  page.on("pageerror", (error) => browserErrors.push(error.message));
  await page.addInitScript(
    (positions) => {
      localStorage.setItem(
        "tailpath.graph-layout.v1",
        JSON.stringify({
          version: 1,
          nodes: positions.map(([id, position]) => ({
            id,
            ...position,
            lastSeen: Date.now(),
          })),
        }),
      );
    },
    [...cachedPositions],
  );
  await page.route("**/api/v1/topology", async (route) => {
    await route.fulfill({
      json: {
        generatedAt: observedAt,
        nodes: [...cachedPositions].map(([id]) => topologyNode(id)),
        edges: [
          topologyEdge("direct-long", "direct-a", "direct-b", "direct"),
          topologyEdge("derp-long", "derp-a", "derp-b", "derp"),
          topologyEdge(
            "blockers-visible",
            "direct-blocker",
            "derp-blocker",
            "direct",
          ),
        ],
        observers: [],
      },
    });
  });
  return browserErrors;
}

function topologyNode(id: string) {
  return {
    id,
    stableNodeId: id,
    hostname: id,
    os: id.endsWith("-a") ? "ios" : "linux",
    observable: id.includes("blocker"),
    online: true,
    lastEvidenceAt: observedAt,
    clockSkewed: false,
  };
}

function topologyEdge(
  id: string,
  source: string,
  target: string,
  kind: "direct" | "derp",
) {
  return {
    id,
    source,
    target,
    path: kind === "derp" ? { kind, derpRegion: "hgh-custom" } : { kind },
    state: "active",
    aToBBytesPerSecond: 24_000,
    bToABytesPerSecond: 8_000,
    lastActive: observedAt,
    observations: [],
  };
}

function canonicalPositions(value: string | null) {
  return new Map(
    (value ?? "")
      .split("|")
      .filter(Boolean)
      .map((entry) => {
        const separator = entry.lastIndexOf(":");
        const [x, y] = entry
          .slice(separator + 1)
          .split(",")
          .map(Number);
        return [entry.slice(0, separator), { x, y }] as const;
      }),
  );
}

function virtualPosition(value: string | null) {
  const entry = (value ?? "")
    .split("|")
    .find((candidate) => candidate.startsWith("derp:hgh-custom:"));
  if (!entry) return null;
  const coordinates = entry.slice("derp:hgh-custom:".length);
  const [x, y] = coordinates.split(",").map(Number);
  return { x, y };
}

function distance(
  left: { x: number; y: number } | null,
  right?: { x: number; y: number },
) {
  if (!left || !right) return 0;
  return Math.hypot(right.x - left.x, right.y - left.y);
}
