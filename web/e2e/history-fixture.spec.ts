import { expect, test } from "@playwright/test";

test("opens seeded history from the real fixture API", async ({
  page,
}, testInfo) => {
  await page.goto("/history?window=1h");
  await expect(page.locator(".history-shell")).toHaveAttribute(
    "data-history-ready",
    "true",
    { timeout: 15_000 },
  );

  const connection = page
    .getByRole("button")
    .filter({ hasText: /MacBook.*iPhone|iPhone.*MacBook/ });
  await expect(connection).toBeVisible();
  await connection.click();
  await expect(
    page.getByRole("heading", { name: /MacBook.*iPhone|iPhone.*MacBook/ }),
  ).toBeVisible();
  await expect(page.locator(".traffic-line-a")).toHaveAttribute("d", /L/);
  await expect(page.getByRole("list", { name: "Path timeline" })).toContainText(
    /DERP|Direct/,
  );

  if (testInfo.project.name.startsWith("mobile")) {
    await expect(page.getByLabel("History connections")).toBeHidden();
    await page.goBack();
    await expect(page.getByLabel("History connections")).toBeVisible();
  }
});
