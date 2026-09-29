import { expect, test, type Page } from "@playwright/test";

const nodes = [
  {
    id: "node-mac",
    stableNodeId: "stable-mac",
    label: "MacBook",
    hostname: "macbook",
    os: "macos",
  },
  {
    id: "node-dev",
    stableNodeId: "stable-dev",
    label: "DevBox",
    hostname: "devbox",
    os: "linux",
  },
  {
    id: "node-phone",
    stableNodeId: "stable-phone",
    label: "iPhone",
    hostname: "iphone",
    os: "ios",
  },
  {
    id: "node-win",
    stableNodeId: "stable-win",
    label: "Windows",
    hostname: "windows",
    os: "windows",
  },
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
  const listLastTraffic = page
    .locator(".history-edge-row")
    .first()
    .locator("time");
  const detailLastTraffic = page.locator(".history-detail-summary time");
  await expect(detailLastTraffic).toHaveAttribute(
    "datetime",
    (await listLastTraffic.getAttribute("datetime")) ?? "",
  );
  await expect(detailLastTraffic).toHaveText(
    (await listLastTraffic.textContent()) ?? "",
  );
  await expect(page.getByLabel(/MacBook to DevBox above zero/)).toBeVisible();
  await expect(page.locator(".traffic-line-a")).toHaveAttribute("d", /L/);
  await expect(page.locator(".traffic-line-b")).toHaveAttribute("d", /L/);
  await expect(page.getByRole("list", { name: "Path timeline" })).toContainText(
    "DERP",
  );
  await expect(page.getByText("Newest first", { exact: true })).toBeVisible();
  await expect(page.getByRole("listitem").first()).toContainText("Direct");
  await page.getByRole("listitem").filter({ hasText: "DERP" }).click();
  await expect(page.getByRole("table", { name: "Observed by" })).toContainText(
    "MacBook",
  );
  await expect(page.getByLabel("Collector clock warning")).toBeVisible();
  await expect(page.getByRole("table", { name: "Observed by" })).toContainText(
    "Supports selected path: DERP hkg",
  );
  await expect(page.getByRole("table", { name: "Observed by" })).toContainText(
    "Conflicts: Direct",
  );

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
  const timeline = page.getByRole("list", { name: "Path timeline" });
  await timeline.scrollIntoViewIfNeeded();
  const detailPane = page.getByLabel("History edge detail");
  const scrollBefore = await detailPane.evaluate(
    (element) => element.scrollTop,
  );
  const newestEvent = timeline.getByRole("listitem").first();
  await newestEvent.click();
  const sheet = page.getByRole("dialog", { name: "Path evidence" });
  await expect(sheet).toBeVisible();
  await expect(sheet.getByText("Effective path at")).toBeVisible();
  await expect(
    sheet.getByRole("button", { name: "Close path evidence" }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(sheet).toBeHidden();
  await expect(newestEvent).toBeFocused();
  expect(await detailPane.evaluate((element) => element.scrollTop)).toBe(
    scrollBefore,
  );
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

test("renders directional history on shared chronological lanes", async ({
  page,
}, testInfo) => {
  const detail = directionalHistoryFor(edgeSummaries[0], 3);
  detail.pathEvents[2].directions[0].clockSkewed = true;
  await page.route(
    "**/api/v1/history/edges/node-mac--node-dev/paths?**",
    (route) =>
      route.fulfill({
        json: {
          source: detail.source,
          target: detail.target,
          relatedNodes: detail.relatedNodes,
          anchor: detail.pathAnchor,
          events: detail.pathEvents,
        },
      }),
  );
  await page.route("**/api/v1/history/edges/node-mac--node-dev?**", (route) =>
    route.fulfill({ json: detail }),
  );
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });

  await page.goto("/history/edges/node-mac--node-dev?window=24h");
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  await expect(
    page.getByText("Complete · 3 events", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Time flows left to right", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Newest first", { exact: true })).toHaveCount(0);
  const timeline = page.getByRole("list", { name: "Path timeline" });
  await expect(timeline.getByRole("listitem")).toHaveCount(4);
  await expect(page.locator(".directional-lane-segment.a-primary")).toHaveCount(
    4,
  );
  await expect(page.locator(".directional-lane-segment.b-primary")).toHaveCount(
    4,
  );
  await expect(page.locator(".directional-fallback-segment")).not.toHaveCount(
    0,
  );
  await expect(page.locator(".chart-selected-cursor")).toHaveCount(1);
  await expect(page.locator(".chart-selected-cursor")).toHaveAttribute(
    "x1",
    /\d/,
  );

  const lastState = timeline.getByRole("listitem").last();
  if (testInfo.project.name.startsWith("mobile")) {
    const touchTarget = await lastState.boundingBox();
    expect(touchTarget?.width ?? 0).toBeGreaterThanOrEqual(44);
    expect(touchTarget?.height ?? 0).toBeGreaterThanOrEqual(44);
    await lastState.click();
    const sheet = page.getByRole("dialog", { name: "Path evidence" });
    await expect(sheet).toContainText("Asymmetric paths");
    await expect(
      sheet.getByRole("table", { name: "Directional path state" }),
    ).toContainText("DERP hkg");
    await expect(sheet).toContainText("Inferred");
    await expect(sheet).toContainText("Observer MacBook");
    await expect(sheet).toContainText("Collected");
    await expect(sheet).toContainText("Received");
    await expect(sheet).toContainText("Resolution: endpoint match");
    await expect(sheet).toContainText("Collector clock warning");
    const retainedEvidence = sheet.locator(".directional-history-evidence");
    await expect(retainedEvidence).toContainText("Relay identity");
    await expect(retainedEvidence).toContainText("Collected");
    await expect(retainedEvidence).toContainText("Received");
    await expect(retainedEvidence).toContainText("Collector clock warning");
    await expect(
      sheet.getByRole("button", { name: "Close path evidence" }),
    ).toBeFocused();
    await page.screenshot({
      path: testInfo.outputPath("directional-history-sheet-mobile.png"),
      fullPage: true,
    });
    await page.keyboard.press("Escape");
    await expect(sheet).toBeHidden();
    await expect(lastState).toBeFocused();
  } else {
    const table = page.getByRole("table", { name: "Directional path state" });
    await expect(table).toContainText("MacBook → DevBox");
    await expect(table).toContainText("DevBox → MacBook");
    await expect(table).toContainText("DERP hkg");
    await expect(table).toContainText("Inferred");
    await expect(table).toContainText("Observer MacBook");
    await expect(table).toContainText("Collected");
    await expect(table).toContainText("Received");
    await expect(table).toContainText("Resolution: endpoint match");
    await expect(table).toContainText("Collector clock warning");
    const retainedEvidence = page.locator(".directional-history-evidence");
    await expect(retainedEvidence).toContainText("Relay identity");
    await expect(retainedEvidence).toContainText("Collected");
    await expect(retainedEvidence).toContainText("Received");
    await expect(retainedEvidence).toContainText("Collector clock warning");
  }

  const labelFailures = await page
    .locator(".directional-lane-segment, .directional-fallback-segment")
    .evaluateAll((segments) =>
      segments.flatMap((segment, index) => {
        if (!segment.textContent?.trim()) return [];
        const style = getComputedStyle(segment);
        return segment.clientWidth >= 55 &&
          segment.scrollHeight <= segment.clientHeight + 1 &&
          style.overflow === "hidden" &&
          style.whiteSpace === "nowrap"
          ? []
          : [`${index}: ${segment.textContent}`];
      }),
    );
  expect(labelFailures).toEqual([]);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  expect(consoleErrors).toEqual([]);

  await page.screenshot({
    path: testInfo.outputPath(
      `directional-history-${testInfo.project.name}.png`,
    ),
    fullPage: true,
  });
});

test("keeps directional evidence within an intermediate desktop width", async ({
  page,
}, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("desktop"));
  await page.setViewportSize({ width: 1000, height: 900 });
  const detail = directionalHistoryFor(edgeSummaries[0], 3);
  const relay = detail.pathEvents[0].directions[0].primaryPath;
  detail.pathEvents[2] = {
    ...detail.pathEvents[2],
    path: relay,
    directions: detail.pathEvents[2].directions.map((direction, index) => ({
      ...direction,
      primaryPath: { ...relay, peerRelayVni: index === 0 ? 4293 : 8 },
      fallbackPath: undefined,
      evidence: "observed" as const,
      inferenceRule: undefined,
    })),
  };
  await page.route(
    "**/api/v1/history/edges/node-mac--node-dev/paths?**",
    (route) =>
      route.fulfill({
        json: {
          source: detail.source,
          target: detail.target,
          relatedNodes: detail.relatedNodes,
          anchor: detail.pathAnchor,
          events: detail.pathEvents,
        },
      }),
  );
  await page.route("**/api/v1/history/edges/node-mac--node-dev?**", (route) =>
    route.fulfill({ json: detail }),
  );

  await page.goto("/history/edges/node-mac--node-dev?window=24h");
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  const table = page.getByRole("table", { name: "Directional path state" });
  await expect(table).toBeVisible();
  await expect(page.locator(".history-detail-summary")).toContainText(
    "Asymmetric paths",
  );
  await expect(page.locator(".directional-snapshot-state")).toHaveText(
    "Asymmetric paths",
  );
  await expect(table).toContainText("VNI 4293");
  await expect(table).toContainText("VNI 8");
  expect(
    await table.evaluate(
      (element) => element.scrollWidth <= element.clientWidth + 1,
    ),
  ).toBe(true);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});

test("loads all 900 directional path events without truncation", async ({
  page,
}, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("desktop"));
  test.setTimeout(30_000);
  const detail = directionalHistoryFor(edgeSummaries[0], 900);
  const direct = { kind: "direct" as const };
  const relay = {
    kind: "peer_relay" as const,
    peerRelayStableNodeId: "relay-hangzhou-stable",
    peerRelayEndpoint: "203.0.113.10:41642",
    peerRelayResolution: "endpoint_match" as const,
    peerRelayVni: 4293,
  };
  detail.pathEvents[499] = {
    ...detail.pathEvents[499],
    path: direct,
    directions: detail.pathEvents[499].directions.map((state) => ({
      ...state,
      primaryPath: direct,
      fallbackPath: undefined,
      inferenceRule: undefined,
      evidence: "observed" as const,
    })),
  };
  detail.pathEvents[899] = {
    ...detail.pathEvents[899],
    path: relay,
    directions: detail.pathEvents[899].directions.map((state) => ({
      ...state,
      primaryPath: relay,
      fallbackPath: undefined,
      inferenceRule: undefined,
      evidence: "observed" as const,
    })),
  };
  let pageRequests = 0;
  let releaseSecondPage: (() => void) | undefined;
  const secondPageGate = new Promise<void>((resolve) => {
    releaseSecondPage = resolve;
  });
  await page.route(
    "**/api/v1/history/edges/node-mac--node-dev/paths?**",
    (route) => {
      pageRequests += 1;
      const query = new URL(route.request().url()).searchParams;
      expect(query.get("to")).toBe(detail.to);
      const cursor = query.get("cursor");
      const events = cursor
        ? detail.pathEvents.slice(500)
        : detail.pathEvents.slice(0, 500);
      return (async () => {
        if (cursor) await secondPageGate;
        await route.fulfill({
          json: {
            source: detail.source,
            target: detail.target,
            relatedNodes: detail.relatedNodes,
            anchor: detail.pathAnchor,
            events,
            nextCursor: cursor ? undefined : "second-page",
          },
        });
      })();
    },
  );
  await page.route("**/api/v1/history/edges/node-mac--node-dev?**", (route) =>
    route.fulfill({
      json: {
        ...detail,
        pathEvents: detail.pathEvents.slice(-500),
        pathEventsTruncated: true,
      },
    }),
  );

  await page.goto("/history/edges/node-mac--node-dev?window=24h");
  await expect(
    page.getByText("Loading path history · 500 events", { exact: true }),
  ).toBeVisible();
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "false",
  );
  await expect(page.getByLabel("History server connecting")).toBeVisible();
  await expect(page.locator(".history-detail-summary")).toContainText(
    "Peer Relay",
  );
  releaseSecondPage?.();
  await expect(
    page.getByText("Complete · 900 events", { exact: true }),
  ).toBeVisible();
  const renderedTimelineStates = page
    .getByRole("list", { name: "Path timeline" })
    .getByRole("listitem");
  expect(await renderedTimelineStates.count()).toBeLessThanOrEqual(240);
  const eventIndex = page.locator(".directional-event-index");
  await eventIndex.locator("summary").click();
  await expect(eventIndex).toContainText("States 802–901 of 901");
  await expect(
    eventIndex.locator(".directional-event-index-list").getByRole("listitem"),
  ).toHaveCount(100);
  await eventIndex.getByRole("button", { name: "Previous states" }).click();
  await expect(eventIndex).toContainText("States 702–801 of 901");
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  await expect(page.getByLabel("History server reachable")).toBeVisible();
  await expect(page.getByText("Latest retained points shown")).toHaveCount(0);
  expect(pageRequests).toBe(2);
});

test("separates mobile History identity, recency, and traffic totals", async ({
  page,
}, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("mobile"));
  await page.setViewportSize({ width: 390, height: 844 });
  const longSummary = {
    ...edgeSummaries[0],
    source: {
      ...edgeSummaries[0].source,
      label: "smallbox-with-a-very-long-hostname",
    },
    target: { ...edgeSummaries[0].target, label: "iphone181" },
  };
  await page.route("**/api/v1/history/edges?**", async (route) => {
    await route.fulfill({ json: { edges: [longSummary] } });
  });

  await page.goto("/history?window=24h");
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  const row = page.locator(".history-edge-row").first();
  const identity = row.locator(".history-row-identity");
  const metadata = row.locator(".history-row-metadata");
  const chevron = row.locator(".history-row-chevron");
  const time = metadata.locator("time");
  const totals = metadata.locator(".history-row-totals > span");
  await expect(row).toBeVisible();

  const [identityBox, metadataBox, chevronBox, timeBox, firstTotalBox] =
    await Promise.all([
      identity.boundingBox(),
      metadata.boundingBox(),
      chevron.boundingBox(),
      time.boundingBox(),
      totals.first().boundingBox(),
    ]);
  if (
    !identityBox ||
    !metadataBox ||
    !chevronBox ||
    !timeBox ||
    !firstTotalBox
  ) {
    throw new Error("mobile History row has incomplete geometry");
  }
  expect(identityBox.x + identityBox.width + 8).toBeLessThanOrEqual(
    metadataBox.x,
  );
  expect(metadataBox.x + metadataBox.width + 8).toBeLessThanOrEqual(
    chevronBox.x,
  );
  expect(timeBox.y + timeBox.height + 4).toBeLessThanOrEqual(firstTotalBox.y);
  expect(
    await identity
      .locator(".history-row-pair")
      .evaluate((element) => element.scrollWidth > element.clientWidth),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("history-mobile-metadata.png"),
    fullPage: true,
  });
});

test("keeps path evidence usable in a 320px bottom sheet", async ({
  page,
}, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("mobile"));
  await page.setViewportSize({ width: 320, height: 700 });
  const relayStableNodeID =
    "nodekey:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";
  const baseDetail = historyFor(edgeSummaries[0]);
  const detail = {
    ...baseDetail,
    pathEvents: baseDetail.pathEvents.map((event, index) =>
      index === baseDetail.pathEvents.length - 1
        ? {
            ...event,
            path: {
              kind: "peer_relay" as const,
              peerRelayStableNodeId: relayStableNodeID,
            },
          }
        : event,
    ),
  };
  await page.route("**/api/v1/history/edges/*?**", async (route) => {
    await route.fulfill({ json: detail });
  });
  await page.route("**/api/v1/history/edges/*/paths?**", async (route) => {
    await route.fulfill({
      json: { anchor: detail.pathAnchor, events: detail.pathEvents },
    });
  });
  await page.goto("/history/edges/node-mac--node-dev?window=24h");
  const timeline = page.getByRole("list", { name: "Path timeline" });
  await timeline.scrollIntoViewIfNeeded();
  const timelinePathLabel = timeline
    .getByRole("listitem")
    .first()
    .locator(".timeline-copy strong");
  await expect(timelinePathLabel).toContainText(relayStableNodeID);
  expect(
    await timelinePathLabel.evaluate(
      (element) => element.scrollWidth <= element.clientWidth,
    ),
  ).toBe(true);
  expect(
    await page.locator("body").evaluate((body) => body.scrollWidth),
  ).toBeLessThanOrEqual(320);
  await timeline.getByRole("listitem").first().click();

  const sheet = page.getByRole("dialog", { name: "Path evidence" });
  await expect(sheet).toBeVisible();
  const bounds = await sheet.boundingBox();
  if (!bounds) throw new Error("path evidence sheet has no bounds");
  expect(bounds.height).toBeLessThanOrEqual(490);
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(320);
  expect(
    await page.locator("body").evaluate((body) => body.scrollWidth),
  ).toBeLessThanOrEqual(320);
  const pathLabel = sheet.getByLabel("Effective path");
  await expect(pathLabel).toContainText(relayStableNodeID);
  expect(
    await pathLabel.evaluate(
      (element) => element.scrollWidth <= element.clientWidth,
    ),
  ).toBe(true);
  await page.keyboard.press("Shift+Tab");
  await expect(
    sheet.getByRole("button", { name: "Close path evidence" }),
  ).toBeFocused();
  await page.screenshot({
    path: testInfo.outputPath("history-evidence-sheet-320.png"),
    fullPage: true,
  });
  await sheet.getByRole("button", { name: "Close path evidence" }).click();
  await expect(sheet).toBeHidden();
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
  const actionBounds = await page
    .getByRole("button", { name: /Next page/ })
    .boundingBox();
  expect(actionBounds!.width).toBeGreaterThanOrEqual(44);
  expect(actionBounds!.height).toBeGreaterThanOrEqual(44);
  await page.getByRole("button", { name: /Next page/ }).click();
  await expect(page).toHaveURL(/cursor=page-2/);
});

test("shows a bounded empty detail when desktop History changes to 15m", async ({
  page,
}, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("desktop"));
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });
  await page.route("**/api/v1/history/edges?**", async (route) => {
    const url = new URL(route.request().url());
    if (url.searchParams.get("window") === "15m") {
      await route.fulfill({ json: { edges: [] } });
      return;
    }
    await route.fallback();
  });
  await page.route("**/api/v1/history/edges/*?**", async (route) => {
    const url = new URL(route.request().url());
    if (url.searchParams.get("window") === "15m") {
      const summary = edgeSummaries[0];
      await route.fulfill({
        json: { ...historyFor(summary), traffic: [], pathEvents: [] },
      });
      return;
    }
    await route.fallback();
  });

  await page.goto("/history?window=24h");
  await expect(page).toHaveURL(/history\/edges\/node-mac--node-dev/);
  await page
    .getByRole("button", { name: "15m", exact: true })
    .filter({ visible: true })
    .click();

  await expect(page).toHaveURL(/window=15m/);
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  await expect(page.getByText("No matching traffic")).toBeVisible();
  await expect(page.locator(".history-detail-empty")).toContainText(
    "No traffic in this window",
  );
  await expect(
    page.getByRole("heading", { name: /MacBook.*DevBox/ }),
  ).toBeVisible();
  expect(consoleErrors).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath("history-15m-empty.png"),
    fullPage: true,
  });
});

test("renders a direct empty-window link with legacy null collections", async ({
  page,
}, testInfo) => {
  const browserErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error" || message.type() === "warning") {
      browserErrors.push(`${message.type()}: ${message.text()}`);
    }
  });
  page.on("pageerror", (error) => browserErrors.push(error.message));
  await page.route("**/api/v1/history/edges/*?**", async (route) => {
    const summary = edgeSummaries[0];
    await route.fulfill({
      json: {
        ...historyFor(summary),
        traffic: null,
        pathAnchor: {
          observedAt: "2026-08-23T01:59:00Z",
          path: { kind: "direct" },
          conflicts: null,
          observations: null,
        },
        relatedNodes: null,
        pathEvents: null,
      },
    });
  });

  await page.goto("/history/edges/node-mac--node-dev?window=15m");

  await expect(page).toHaveTitle("Tailpath");
  await expect(page).toHaveURL(
    /history\/edges\/node-mac--node-dev\?window=15m/,
  );
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  await expect(
    page.getByRole("heading", { name: /MacBook.*DevBox/ }),
  ).toBeVisible();
  await expect(page.locator(".history-detail-empty")).toContainText(
    "No traffic in this window",
  );
  await expect(page.locator("body")).not.toBeEmpty();
  await page
    .getByRole("button", { name: "1h", exact: true })
    .filter({ visible: true })
    .click();
  await expect(page).toHaveURL(/window=1h/);
  await expect(page.locator(".history-detail-empty")).toContainText(
    "No traffic in this window",
  );
  expect(browserErrors).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath(
      `history-null-collections-${testInfo.project.name}.png`,
    ),
    fullPage: true,
  });
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
  await page.route("**/api/v1/history/edges/*/paths?**", async (route) => {
    const segments = new URL(route.request().url()).pathname.split("/");
    const edgeID = decodeURIComponent(segments.at(-2) ?? "");
    const summary = edgeSummaries.find((edge) => edge.edgeId === edgeID);
    if (!summary) {
      await route.fulfill({ status: 404, body: "unknown edge" });
      return;
    }
    const history = historyFor(summary);
    await route.fulfill({
      json: {
        anchor: history.pathAnchor,
        events: history.pathEvents,
      },
    });
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
  const provenance = (
    kind: "direct" | "derp",
    clockSkewed = false,
    observerId = summary.source.id,
  ) => ({
    observerId,
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
    systemTelemetry: false,
    relatedNodes: [summary.source, summary.target],
    from: new Date(from).toISOString(),
    to: new Date(to).toISOString(),
    bucketDurationMs: 12 * 60_000,
    lastTrafficAt: summary.lastTrafficAt,
    traffic,
    pathAnchor: {
      observedAt: new Date(from - 60_000).toISOString(),
      path: { kind: "direct", directEndpoint: "203.0.113.4:41641" },
      conflicts: [],
      observations: [provenance("direct")],
    },
    pathEvents: [
      {
        observedAt: "2026-08-23T14:00:00Z",
        path: { kind: "derp", derpRegion: "hkg" },
        conflicts: [{ kind: "direct" }],
        observations: [
          provenance("derp", true),
          provenance("direct", false, summary.target.id),
        ],
      },
      {
        observedAt: "2026-08-23T20:00:00Z",
        path: { kind: "direct", directEndpoint: "203.0.113.4:41641" },
        conflicts: [],
        observations: [provenance("direct")],
      },
    ],
    trafficTruncated: false,
    pathEventsTruncated: false,
  };
}

function directionalHistoryFor(
  summary: (typeof edgeSummaries)[number],
  eventCount: number,
) {
  const base = historyFor(summary);
  const relay = {
    kind: "peer_relay" as const,
    peerRelayStableNodeId: "relay-hangzhou-stable",
    peerRelayEndpoint: "203.0.113.10:41642",
    peerRelayResolution: "endpoint_match" as const,
    peerRelayVni: 4293,
  };
  const reverseRelay = { ...relay, peerRelayVni: 8 };
  const direct = {
    kind: "direct" as const,
    directEndpoint: "198.51.100.20:41641",
  };
  const fallback = { kind: "derp" as const, derpRegion: "hkg" };
  const direction = (
    fromNodeId: string,
    toNodeId: string,
    primaryPath: typeof relay | typeof direct,
    fallbackPath?: typeof fallback,
  ) => ({
    fromNodeId,
    toNodeId,
    primaryPath,
    fallbackPath,
    evidence: fallbackPath ? ("inferred" as const) : ("observed" as const),
    inferenceRule: fallbackPath ? "tailscale-status-fallback-v1" : undefined,
    observerId: fromNodeId,
    collectedAt: base.from,
    receivedAt: base.from,
    clockSkewed: false,
  });
  const eventAt = (index: number) =>
    new Date(
      Date.parse(base.from) +
        ((Date.parse(base.to) - Date.parse(base.from)) * (index + 1)) /
          (eventCount + 1),
    ).toISOString();
  const pathEvents = Array.from({ length: eventCount }, (_, index) => {
    const observedAt = eventAt(index);
    const aToB =
      index % 3 === 2
        ? direction(summary.source.id, summary.target.id, direct)
        : direction(
            summary.source.id,
            summary.target.id,
            relay,
            index % 3 === 0 ? fallback : undefined,
          );
    const bToA = direction(
      summary.target.id,
      summary.source.id,
      index % 3 === 0 ? direct : reverseRelay,
      index % 3 === 2 ? fallback : undefined,
    );
    return {
      observedAt,
      path: aToB.primaryPath,
      pathState: "stable" as const,
      conflicts: [],
      observations: [
        {
          observerId: summary.source.id,
          path: aToB.primaryPath,
          fallbackPath: aToB.fallbackPath,
          pathEvidence: aToB.evidence,
          pathInferenceRule: aToB.inferenceRule,
          collectedAt: observedAt,
          receivedAt: observedAt,
          clockSkewed: false,
        },
        {
          observerId: summary.target.id,
          path: bToA.primaryPath,
          fallbackPath: bToA.fallbackPath,
          pathEvidence: bToA.evidence,
          pathInferenceRule: bToA.inferenceRule,
          collectedAt: observedAt,
          receivedAt: observedAt,
          clockSkewed: false,
        },
        {
          observerId: "relay-hangzhou",
          path: relay,
          collectedAt: new Date(Date.parse(observedAt) + 1_000).toISOString(),
          receivedAt: new Date(Date.parse(observedAt) + 3_000).toISOString(),
          clockSkewed: index === eventCount - 1,
          relaySession: {
            sessionId: `relay-session-${index}`,
            vni: relay.peerRelayVni,
            sourceIdentityStatus: "resolved" as const,
            targetIdentityStatus: "resolved" as const,
          },
        },
      ],
      directions: [aToB, bToA],
    };
  });
  const anchorDirections = [
    direction(summary.source.id, summary.target.id, relay, fallback),
    direction(summary.target.id, summary.source.id, direct),
  ];
  return {
    ...base,
    relatedNodes: [
      ...base.relatedNodes,
      {
        id: "relay-hangzhou",
        stableNodeId: "relay-hangzhou-stable",
        label: "aliyun-hangzhou-relay",
      },
    ],
    pathAnchor: {
      observedAt: new Date(Date.parse(base.from) - 60_000).toISOString(),
      path: relay,
      conflicts: [],
      observations: [],
      directions: anchorDirections,
    },
    pathEvents,
    pathEventsTruncated: eventCount > 500,
  };
}
