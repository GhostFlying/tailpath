import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";

test.skip(
  process.env.TAILPATH_SCALE_E2E !== "1",
  "the 1,000-edge browser baseline is workflow_dispatch only",
);

test("renders the deterministic 250-node/1,000-edge fixture", async ({
  page,
}, testInfo) => {
  test.setTimeout(180_000);
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });

  const startedAt = Date.now();
  await page.goto("/");
  await expect(page.getByText("Tailpath")).toBeVisible();
  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toHaveAttribute("data-edge-count", "1000", {
    timeout: 120_000,
  });
  await expect(graph).toHaveAttribute("data-node-count", "505");
  await expect(graph).toHaveAttribute("data-ready", "true", {
    timeout: 120_000,
  });
  await expect(graph).toHaveAttribute("data-device-nodes-square", "true");
  await expect(graph).toHaveAttribute("data-layout-runs", "1");
  const firstPositions = await graph.getAttribute("data-layout-positions");

  const topology = await page.evaluate(async () => {
    const response = await fetch("/api/v1/topology");
    return (await response.json()) as {
      nodes: unknown[];
      edges: Array<{ path: { kind: string }; state: string }>;
      observers: Array<{ clockSkewed: boolean }>;
    };
  });
  expect(topology.nodes).toHaveLength(250);
  expect(topology.edges).toHaveLength(1000);
  expect(topology.observers).toHaveLength(250);
  expect(new Set(topology.edges.map((edge) => edge.path.kind))).toEqual(
    new Set(["direct", "derp", "peer_relay", "unknown"]),
  );
  expect(topology.edges.filter((edge) => edge.state === "active")).toHaveLength(
    666,
  );
  expect(topology.edges.filter((edge) => edge.state === "recent")).toHaveLength(
    334,
  );
  expect(
    topology.observers.filter((observer) => observer.clockSkewed),
  ).toHaveLength(9);
  expect(consoleErrors).toEqual([]);

  const readyElapsedMs = Date.now() - startedAt;
  if (testInfo.project.name === "desktop-chromium") {
    expect(readyElapsedMs).toBeLessThanOrEqual(5_000);
  }
  let visibleUpdateElapsedMs: number | null = null;
  let topologyResponseElapsedMs: number | null = null;
  if (testInfo.project.name === "desktop-chromium") {
    await expect(page.locator(".live-state")).toHaveText("live");
    const layoutRunsBeforeUpdate = await graph.getAttribute("data-layout-runs");
    const positionsBeforeUpdate = await graph.getAttribute(
      "data-layout-positions",
    );
    const viewportBeforeUpdate = await graph.getAttribute("data-viewport");
    const rateSignatureBeforeUpdate = await graph.getAttribute(
      "data-edge-rate-signature",
    );
    const updateStartedAt = Date.now();
    const topologyResponse = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/topology") &&
        response.request().method() === "GET",
    );
    const updateStatus = await page.evaluate(async () => {
      const response = await fetch("/api/v1/fixture/edge-update", {
        method: "POST",
      });
      return response.status;
    });
    expect(updateStatus).toBe(202);
    await topologyResponse;
    topologyResponseElapsedMs = Date.now() - updateStartedAt;
    await expect
      .poll(() => graph.getAttribute("data-edge-rate-signature"), {
        timeout: 500,
        intervals: [25, 50, 75],
      })
      .not.toBe(rateSignatureBeforeUpdate);
    visibleUpdateElapsedMs = Date.now() - updateStartedAt;
    expect(visibleUpdateElapsedMs).toBeLessThanOrEqual(500);
    await expect(graph).toHaveAttribute(
      "data-layout-runs",
      layoutRunsBeforeUpdate ?? "",
    );
    await expect(graph).toHaveAttribute(
      "data-layout-positions",
      positionsBeforeUpdate ?? "",
    );
    await expect(graph).toHaveAttribute(
      "data-viewport",
      viewportBeforeUpdate ?? "",
    );
  }
  const reloadStartedAt = Date.now();
  await page.reload();
  await expect(graph).toHaveAttribute("data-ready", "true", {
    timeout: 30_000,
  });
  const cachedReadyElapsedMs = Date.now() - reloadStartedAt;
  await expect(graph).toHaveAttribute("data-layout-runs", "0");
  expect(await graph.getAttribute("data-layout-positions")).toBe(
    firstPositions,
  );
  const browserMetrics = JSON.stringify(
    {
      project: testInfo.project.name,
      readyElapsedMs,
      cachedReadyElapsedMs,
      topologyResponseElapsedMs,
      visibleUpdateElapsedMs,
      topologyNodes: topology.nodes.length,
      logicalEdges: topology.edges.length,
      renderedNodes: 505,
      consoleErrors,
    },
    null,
    2,
  );
  await writeFile(testInfo.outputPath("scale-browser.json"), browserMetrics);
  await testInfo.attach("scale-browser.json", {
    body: browserMetrics,
    contentType: "application/json",
  });
  await page.screenshot({
    path: testInfo.outputPath("tailpath-scale.png"),
    fullPage: true,
  });
});
