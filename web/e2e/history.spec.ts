import { expect, test, type Page } from "@playwright/test";

const nodes = [
  { id: "node-mac", label: "MacBook", hostname: "macbook", os: "macos" },
  { id: "node-dev", label: "DevBox", hostname: "devbox", os: "linux" },
  { id: "node-phone", label: "iPhone", hostname: "iphone", os: "ios" },
  { id: "node-win", label: "Windows", hostname: "windows", os: "windows" },
];

const edgeSummaries = [
  {
    edgeId: "node-mac--node-dev",
    source: nodes[0],
    target: nodes[1],
    lastTrafficAt: "2026-08-24T01:58:00Z",
    aToBBytes: 1_923_481_600,
    bToABytes: 612_368_384,
    paths: ["direct", "derp"],
  },
  {
    edgeId: "node-mac--node-phone",
    source: nodes[0],
    target: nodes[2],
    lastTrafficAt: "2026-08-24T01:51:00Z",
    aToBBytes: 84_934_656,
    bToABytes: 21_708_800,
    paths: ["derp"],
  },
  {
    edgeId: "node-dev--node-win",
    source: nodes[1],
    target: nodes[3],
    lastTrafficAt: "2026-08-24T01:42:00Z",
    aToBBytes: 18_243_584,
    bToABytes: 7_340_032,
    paths: ["peer_relay"],
  },
] as const;

test.beforeEach(async ({ page }) => {
  await installHistoryAPI(page);
});

test("renders and filters the desktop history workspace", async ({
  page,
}, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("desktop"));
  await page.setViewportSize({ width: 1586, height: 992 });
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });

  await page.goto("/history?window=24h");
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  await expect(page.getByLabel("History server reachable")).toBeVisible();
  await expect(page).toHaveURL(/history\/edges\/node-mac--node-dev/);
  await expect(
    page.getByRole("heading", { name: /MacBook.*DevBox/ }),
  ).toBeVisible();
  await expect(page.getByLabel(/MacBook to DevBox above zero/)).toBeVisible();
  await expect(page.locator(".traffic-line-a")).toHaveAttribute("d", /L/);
  await expect(page.locator(".traffic-line-b")).toHaveAttribute("d", /L/);
  await expect(page.getByRole("list", { name: "Path timeline" })).toContainText(
    "DERP",
  );
  await page.getByRole("listitem").filter({ hasText: "DERP" }).click();
  await expect(page.getByRole("table", { name: "Observed by" })).toContainText(
    "MacBook",
  );
  await expect(page.getByLabel("Collector clock warning")).toBeVisible();

  await page.screenshot({
    path: testInfo.outputPath("history-desktop.png"),
    fullPage: true,
  });

  await page.getByLabel("Path seen").selectOption("derp");
  await expect(page).toHaveURL(/path=derp/);
  await expect(
    page.getByRole("button").filter({ hasText: /MacBook.*iPhone/ }),
  ).toBeVisible();
  await page.getByLabel("Find node").fill("Dev");
  await page.getByRole("option", { name: /DevBox/ }).click();
  await expect(page).toHaveURL(/nodeId=node-dev/);
  await expect(page).not.toHaveURL(/cursor=/);

  expect(consoleErrors).toEqual([]);
});

test("uses list and full-screen detail on mobile", async ({
  page,
}, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("mobile"));
  await page.setViewportSize({ width: 426, height: 922 });

  await page.goto("/history?window=6h");
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  await expect(page.getByLabel("History connections")).toBeVisible();
  await expect(page.getByLabel("History edge detail")).toBeHidden();
  await expect(page.locator(".live-state")).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "History window" }),
  ).toHaveValue("6h");
  await page.screenshot({
    path: testInfo.outputPath("history-mobile-list.png"),
    fullPage: true,
  });

  await page
    .getByRole("button")
    .filter({ hasText: /MacBook.*DevBox/ })
    .click();
  await expect(page).toHaveURL(/history\/edges\/node-mac--node-dev\?window=6h/);
  await expect(page.getByLabel("History connections")).toBeHidden();
  await expect(page.getByLabel("History edge detail")).toBeVisible();
  await expect(
    page.getByRole("heading", { name: /MacBook.*DevBox/ }),
  ).toBeVisible();
  await expect(
    page.getByLabel("Server reachable", { exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("history-mobile-detail.png"),
    fullPage: true,
  });

  await page.getByRole("button", { name: "Back to connections" }).click();
  await expect(page).toHaveURL(/\/history\?window=6h$/);
  await expect(page.getByLabel("History connections")).toBeVisible();
  await page
    .getByRole("button")
    .filter({ hasText: /MacBook.*DevBox/ })
    .click();
  await page.goBack();
  await expect(page.getByLabel("History connections")).toBeVisible();
});

test("shows bounded empty and error states", async ({ page }, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("desktop"));
  let unknownFailures = 0;
  await page.route("**/api/v1/history/edges?**", async (route) => {
    const url = new URL(route.request().url());
    if (url.searchParams.get("path") === "unknown") {
      if (unknownFailures === 0) {
        unknownFailures += 1;
        await route.fulfill({ status: 500, body: "unavailable" });
        return;
      }
      await route.fallback();
      return;
    }
    if (url.searchParams.get("path") === "direct") {
      await route.fulfill({ json: { edges: [] } });
      return;
    }
    await route.fallback();
  });
  await page.goto("/history?path=direct");
  await expect(page.getByText("No matching traffic")).toBeVisible();
  await page.getByLabel("Path seen").selectOption("unknown");
  await expect(page.getByText("History unavailable")).toBeVisible();
  await expect(page.getByLabel("History server unavailable")).toBeVisible();
  await page.getByRole("button", { name: "Retry" }).click();
  await expect(page.getByText("No matching traffic")).toBeVisible();
  await expect(page.getByLabel("History server reachable")).toBeVisible();
});

test("keeps pagination in the URL", async ({ page }, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("desktop"));
  await page.goto("/history");
  await page.getByRole("button", { name: /Next page/ }).click();
  await expect(page).toHaveURL(/cursor=page-2/);
});

test("keeps sparse traffic at real times and leaves gaps inert", async ({
  page,
}, testInfo) => {
  await page.route("**/api/v1/history/edges/*?**", async (route) => {
    const summary = edgeSummaries[0];
    await route.fulfill({
      json: {
        ...historyFor(summary),
        from: "2026-08-24T00:00:00Z",
        to: "2026-08-24T01:00:00Z",
        bucketDurationMs: 10_000,
        traffic: [
          {
            bucketStart: "2026-08-24T00:10:00Z",
            aToBBytes: 100,
            bToABytes: 50,
          },
          {
            bucketStart: "2026-08-24T00:50:00Z",
            aToBBytes: 200,
            bToABytes: 80,
          },
        ],
      },
    });
  });
  await page.goto("/history/edges/node-mac--node-dev?window=1h");
  const chart = page.locator(".traffic-chart");
  await expect(chart).toBeVisible();
  const path = await page.locator(".traffic-line-a").getAttribute("d");
  expect(path?.match(/M/g)).toHaveLength(2);

  const bounds = await chart.boundingBox();
  expect(bounds).not.toBeNull();
  await chart.hover({
    position: { x: bounds!.width / 2, y: bounds!.height / 2 },
  });
  await expect(page.locator(".traffic-tooltip")).toBeHidden();
  await chart.hover({
    position: {
      x: bounds!.width * ((10 * 60 + 5) / (60 * 60)),
      y: bounds!.height / 2,
    },
  });
  await expect(page.locator(".traffic-tooltip")).toBeVisible();
  await expect(page.locator(".traffic-tooltip")).toContainText("10 B/s");
  await page.screenshot({
    path: testInfo.outputPath("history-sparse-traffic.png"),
    fullPage: true,
  });
});

async function installHistoryAPI(page: Page) {
  await page.route("**/api/v1/history/nodes?**", async (route) => {
    await route.fulfill({ json: { nodes } });
  });
  await page.route("**/api/v1/history/edges?**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.searchParams.get("path");
    const nodeID = url.searchParams.get("nodeId");
    const filtered = edgeSummaries.filter(
      (edge) =>
        (!path || edge.paths.includes(path as never)) &&
        (!nodeID || edge.source.id === nodeID || edge.target.id === nodeID),
    );
    await route.fulfill({ json: { edges: filtered, nextCursor: "page-2" } });
  });
  await page.route("**/api/v1/history/edges/*?**", async (route) => {
    const edgeID = decodeURIComponent(
      new URL(route.request().url()).pathname.split("/").at(-1) ?? "",
    );
    const summary = edgeSummaries.find((edge) => edge.edgeId === edgeID);
    if (!summary) {
      await route.fulfill({ status: 404, body: "unknown edge" });
      return;
    }
    await route.fulfill({ json: historyFor(summary) });
  });
}

function historyFor(summary: (typeof edgeSummaries)[number]) {
  const from = Date.parse("2026-08-23T02:00:00Z");
  const to = Date.parse("2026-08-24T02:00:00Z");
  const traffic = Array.from({ length: 121 }, (_, index) => ({
    bucketStart: new Date(from + index * 12 * 60_000).toISOString(),
    aToBBytes: 600_000 + Math.round((Math.sin(index / 8) + 1.2) * 9_000_000),
    bToABytes: 350_000 + Math.round((Math.cos(index / 11) + 1.2) * 3_000_000),
  }));
  const provenance = (kind: "direct" | "derp", clockSkewed = false) => ({
    observerId: summary.source.id,
    path:
      kind === "derp"
        ? { kind, derpRegion: "hkg" }
        : { kind, directEndpoint: "203.0.113.4:41641" },
    collectedAt: "2026-08-23T14:00:00Z",
    receivedAt: "2026-08-23T14:06:00Z",
    clockSkewed,
  });
  return {
    edgeId: summary.edgeId,
    source: summary.source,
    target: summary.target,
    from: new Date(from).toISOString(),
    to: new Date(to).toISOString(),
    bucketDurationMs: 12 * 60_000,
    traffic,
    pathAnchor: {
      observedAt: new Date(from - 60_000).toISOString(),
      path: { kind: "direct", directEndpoint: "203.0.113.4:41641" },
      observations: [provenance("direct")],
    },
    pathEvents: [
      {
        observedAt: "2026-08-23T14:00:00Z",
        path: { kind: "derp", derpRegion: "hkg" },
        observations: [provenance("derp", true)],
      },
      {
        observedAt: "2026-08-23T20:00:00Z",
        path: { kind: "direct", directEndpoint: "203.0.113.4:41641" },
        observations: [provenance("direct")],
      },
    ],
    trafficTruncated: false,
    pathEventsTruncated: false,
  };
}
