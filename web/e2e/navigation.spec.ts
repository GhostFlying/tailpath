import { expect, test, type Locator } from "@playwright/test";

test("keeps mobile workspace tab geometry stable across routes", async ({
  page,
}, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("mobile"));
  await page.setViewportSize({ width: 390, height: 844 });
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });

  await page.goto("/");
  await expect(page.locator(".topbar")).toBeVisible();
  const liveBoxes = await tabBoxes(page.locator(".workspace-tabs"));
  await expect(page.locator(".live-state > span")).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("mobile-tabs-live.png"),
    fullPage: true,
  });

  await page.getByRole("link", { name: "History" }).click();
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
  );
  const historyBoxes = await tabBoxes(page.locator(".workspace-tabs"));
  expectBoxesClose(historyBoxes.live, liveBoxes.live);
  expectBoxesClose(historyBoxes.history, liveBoxes.history);
  await page.screenshot({
    path: testInfo.outputPath("mobile-tabs-history.png"),
    fullPage: true,
  });

  await page.getByRole("link", { name: "Live", exact: true }).click();
  await expect(page.locator(".workspace-tabs")).toBeVisible();
  const restoredBoxes = await tabBoxes(page.locator(".workspace-tabs"));
  expectBoxesClose(restoredBoxes.live, liveBoxes.live);
  expectBoxesClose(restoredBoxes.history, liveBoxes.history);
  expect(consoleErrors).toEqual([]);
});

async function tabBoxes(tabs: Locator) {
  const live = await tabs.getByRole("link", { name: "Live" }).boundingBox();
  const history = await tabs
    .getByRole("link", { name: "History" })
    .boundingBox();
  if (!live || !history) throw new Error("workspace tabs have no bounds");
  return { live, history };
}

function expectBoxesClose(
  actual: { x: number; y: number; width: number; height: number },
  expected: { x: number; y: number; width: number; height: number },
) {
  expect(Math.abs(actual.x - expected.x)).toBeLessThanOrEqual(0.5);
  expect(Math.abs(actual.y - expected.y)).toBeLessThanOrEqual(0.5);
  expect(Math.abs(actual.width - expected.width)).toBeLessThanOrEqual(0.5);
  expect(Math.abs(actual.height - expected.height)).toBeLessThanOrEqual(0.5);
}
