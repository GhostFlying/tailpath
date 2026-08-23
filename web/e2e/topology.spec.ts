import { expect, test } from "@playwright/test";

test("keeps traffic empty states consistent with the recent option", async ({
  page,
}, testInfo) => {
  await page.route("**/api/v1/topology", async (route) => {
    await route.fulfill({
      json: {
        generatedAt: "2026-08-23T00:00:00Z",
        nodes: [
          {
            id: "runtime-a",
            stableNodeId: "runtime-a",
            hostname: "Runtime-A",
            observable: true,
            online: true,
            lastEvidenceAt: "2026-08-23T00:00:00Z",
            clockSkewed: false,
          },
        ],
        edges: [],
        observers: [],
      },
    });
  });

  await page.goto("/");
  const graph = page.getByLabel("Live Tailnet topology");
  const recentSwitch = page.getByRole("switch", { name: "Show recent" });
  await expect(
    page.getByText("No recent traffic", { exact: true }),
  ).toBeVisible();
  await expect(graph).toHaveAttribute("data-edge-count", "0");
  await expect(graph).toHaveAttribute("data-node-count", "0");

  await recentSwitch.click();
  await expect(recentSwitch).toHaveAttribute("aria-checked", "false");
  await expect(
    page.getByText("No active traffic", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("No recent traffic", { exact: true }),
  ).toBeHidden();
  await expect(graph).toHaveAttribute("data-edge-count", "0");
  await expect(graph).toHaveAttribute("data-node-count", "0");

  await page.screenshot({
    path: testInfo.outputPath("tailpath-empty-traffic.png"),
    fullPage: true,
  });
});

test("renders the live fixture topology without overlap", async ({
  page,
}, testInfo) => {
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });
  await page.goto("/");
  await expect(page.getByText("Tailpath")).toBeVisible();
  if (testInfo.project.name.startsWith("desktop")) {
    await expect(page.getByText("active edges")).toBeVisible();
  } else {
    await expect(page.getByText("active edges")).toBeHidden();
  }
  await expect(page.locator(".live-state")).toContainText("live");

  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toBeVisible();
  await expect(graph).toHaveAttribute("data-ready", "true");
  const legend = page.getByLabel("Topology legend");
  await expect(legend).toBeVisible();
  await expect(legend).toContainText("Direct");
  await expect(legend).toContainText("Peer Relay");
  await expect(legend).toContainText("Activity");
  await expect(legend).toContainText("Runtime telemetry");
  await expect(legend).toContainText("Platform device");
  await expect(legend).toContainText("Clock skew");
  await expect(legend).not.toContainText("Observer");
  const recentSwitch = page.getByRole("switch", { name: "Show recent" });
  await expect(recentSwitch).toHaveAttribute("aria-checked", "true");
  const box = await graph.boundingBox();
  expect(box?.width).toBeGreaterThan(300);
  expect(box?.height).toBeGreaterThan(300);
  await expect(graph).toHaveAttribute("data-device-nodes-square", "true");
  expect(
    Number(await graph.getAttribute("data-device-node-count")),
  ).toBeGreaterThan(0);

  const stageBox = await page.locator(".graph-stage").boundingBox();
  const legendBox = await legend.boundingBox();
  expect(legendBox?.x).toBeGreaterThanOrEqual(stageBox?.x ?? 0);
  expect(legendBox?.y).toBeGreaterThanOrEqual(stageBox?.y ?? 0);
  expect((legendBox?.x ?? 0) + (legendBox?.width ?? 0)).toBeLessThanOrEqual(
    (stageBox?.x ?? 0) + (stageBox?.width ?? 0),
  );
  expect((legendBox?.y ?? 0) + (legendBox?.height ?? 0)).toBeLessThanOrEqual(
    (stageBox?.y ?? 0) + (stageBox?.height ?? 0),
  );

  if (testInfo.project.name.startsWith("desktop")) {
    await expect(page.locator(".mobile-path-select")).toBeHidden();
    for (const path of ["Direct", "DERP", "Peer Relay", "Unknown"]) {
      const button = page
        .locator(".path-filter button")
        .filter({ hasText: path });
      await button.click();
      await expect(graph).toHaveAttribute("data-ready", "true");
      await expect(graph).toHaveAttribute(
        "data-edge-count",
        (await button.locator("small").innerText()).trim(),
      );
    }
    await page
      .locator(".path-filter button")
      .filter({ hasText: "Peer Relay" })
      .click();
  } else {
    await expect(page.locator(".path-filter")).toBeHidden();
    await page.getByRole("button", { name: "Open node search" }).click();
    await expect(page.getByLabel("Find node")).toBeVisible();
    await page.getByLabel("Find node").fill("MacBook");
    await page.getByLabel("Find node").fill("");
    for (const path of ["direct", "derp", "peer_relay", "unknown"]) {
      await page.getByLabel("Path filter").selectOption(path);
      await expect(graph).toHaveAttribute("data-ready", "true");
      const option = await page.locator(`option[value="${path}"]`).innerText();
      await expect(graph).toHaveAttribute(
        "data-edge-count",
        option.match(/(\d+)$/)?.[1] ?? "",
      );
    }
    await page.getByLabel("Path filter").selectOption("peer_relay");
  }
  await expect(graph).toHaveAttribute("data-edge-count", "1");
  await recentSwitch.click();
  await expect(recentSwitch).toHaveAttribute("aria-checked", "false");
  await page.reload();
  await expect(
    page.getByRole("switch", { name: "Show recent" }),
  ).toHaveAttribute("aria-checked", "false");
  await page.getByRole("switch", { name: "Show recent" }).click();
  await expect(
    page.getByRole("switch", { name: "Show recent" }),
  ).toHaveAttribute("aria-checked", "true");

  await expect(page.locator("canvas")).toHaveCount(3);
  expect(consoleErrors).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath("tailpath-topology.png"),
    fullPage: true,
  });
});
