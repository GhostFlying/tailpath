import { expect, test } from "@playwright/test";

for (const width of [320, 390]) {
  test(`workspace controls and metadata remain usable at ${width}px`, async ({
    page,
  }, info) => {
    await page.setViewportSize({ width, height: 844 });
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    let tabs: { x: number; width: number }[] | undefined;
    for (const path of ["/", "/history", "/devices"]) {
      await page.goto(path);
      await expect(page.locator(".topbar")).toBeVisible();
      if (path === "/")
        await expect(page.getByLabel("Live Tailnet topology")).toHaveAttribute(
          "data-ready",
          "true",
        );
      else
        await expect(
          page.locator(
            path === "/history" ? ".history-shell" : ".devices-shell",
          ),
        ).toHaveAttribute(
          path === "/history" ? "data-history-ready" : "data-devices-ready",
          "true",
        );
      const current = await page
        .locator(".workspace-tabs a")
        .evaluateAll((elements) =>
          elements.map((e) => {
            const r = e.getBoundingClientRect();
            return { x: r.x, width: r.width };
          }),
        );
      expect(current).toHaveLength(3);
      if (tabs)
        current.forEach((r, i) => {
          expect(Math.abs(r.x - tabs![i].x)).toBeLessThan(1);
          expect(Math.abs(r.width - tabs![i].width)).toBeLessThan(1);
        });
      tabs = current;
      const controls = await page
        .locator(
          ".topbar a, .filters button, .filters select, .history-filters select, .devices-filters select",
        )
        .evaluateAll((elements) =>
          elements
            .filter((e) => e.getClientRects().length)
            .map((e) => ({
              name: e.getAttribute("aria-label") ?? e.textContent,
              rect: e.getBoundingClientRect().toJSON(),
            })),
        );
      for (const { name, rect } of controls) {
        expect(rect.width, `${path}: ${name} width`).toBeGreaterThanOrEqual(44);
        expect(rect.height, `${path}: ${name} height`).toBeGreaterThanOrEqual(
          44,
        );
        expect(rect.x, `${name} left`).toBeGreaterThanOrEqual(0);
        expect(rect.right, `${name} right`).toBeLessThanOrEqual(width + 1);
      }
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth + 1,
      );
      expect(overflow).toBe(false);
      if (width === 320 && path === "/devices") {
        const row = page.locator(".device-row").first();
        const identity = await row
          .locator(".device-identity > span")
          .boundingBox();
        const status = await row.locator(".device-control").boundingBox();
        expect(identity!.y + identity!.height).toBeLessThanOrEqual(status!.y);
      }
      await page.screenshot({
        path: info.outputPath(
          `synthetic-workspace-${path.slice(1) || "live"}-${width}.png`,
        ),
        fullPage: true,
      });
    }
    expect(errors).toEqual([]);
  });
}

test("mobile retry actions meet the touch target and recover", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 844 });
  for (const [path, api] of [
    ["/", "**/api/v1/topology"],
    ["/history", "**/api/v1/history/edges?**"],
    ["/devices", "**/api/v1/devices"],
  ]) {
    await page.route(api, (route) =>
      route.fulfill({ status: 503, body: "unavailable" }),
    );
    await page.goto(path);
    const retry = page
      .getByRole("button", { name: "Retry", exact: true })
      .first();
    await expect(retry).toBeVisible();
    const bounds = await retry.boundingBox();
    expect(bounds!.width).toBeGreaterThanOrEqual(44);
    expect(bounds!.height).toBeGreaterThanOrEqual(44);
    await page.unroute(api);
    await retry.click();
    await expect(retry).toBeHidden();
  }
});
