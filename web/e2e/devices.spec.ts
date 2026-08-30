import { expect, test, type Page } from "@playwright/test";

test("renders the real 250-device directory without changing Live topology", async ({
  page,
}, testInfo) => {
  const browserErrors = captureBrowserErrors(page);
  await page.goto("/devices");
  await expect(page.locator(".devices-shell")).toHaveAttribute(
    "data-devices-ready",
    "true",
  );
  await expect(page.getByRole("table", { name: "Devices" })).toBeVisible();
  await expect(page.locator(".devices-summary")).toContainText(/250\s*devices/);

  if (testInfo.project.name.startsWith("desktop")) {
    await expect(
      page.getByLabel("Device detail", { exact: true }),
    ).toBeVisible();
    await page.screenshot({
      path: testInfo.outputPath("devices-desktop.png"),
      fullPage: true,
    });
  }

  await page.getByRole("link", { name: "Live", exact: true }).click();
  const graph = page.getByLabel("Live Tailnet topology");
  const topology = await page.evaluate(async () => {
    const response = await fetch("/api/v1/topology");
    return (await response.json()) as { nodes: unknown[]; edges: unknown[] };
  });
  expect(topology.nodes.length).toBeGreaterThan(0);
  expect(topology.nodes.length).toBeLessThan(250);
  expect(topology.edges.length).toBeGreaterThan(0);
  expect(
    topology.nodes.some((node) =>
      JSON.stringify(node).includes("directory-only"),
    ),
  ).toBe(false);
  await expect
    .poll(async () =>
      Number(await graph.getAttribute("data-device-node-count")),
    )
    .toBeLessThan(250);
  expect(browserErrors).toEqual([]);
});

test("keeps device filters in the URL and combines them client-side", async ({
  page,
}) => {
  await page.goto("/devices");
  await expect(page.locator(".devices-summary")).toContainText(/250\s*devices/);

  const expected = await page.evaluate(async () => {
    const response = await fetch("/api/v1/devices");
    const directory = (await response.json()) as {
      devices: Array<{
        dnsName?: string;
        hostname?: string;
        platform?: string;
        connectedToControl: boolean;
      }>;
    };
    const linux = directory.devices.filter(
      (device) => device.platform === "linux",
    );
    const disconnected = linux.filter((device) => !device.connectedToControl);
    const named = disconnected.filter((device) =>
      [device.dnsName, device.hostname].some((value) =>
        value?.includes("catalog-node-016"),
      ),
    );
    return {
      linux: linux.length,
      disconnected: disconnected.length,
      named: named.length,
    };
  });

  await page.getByLabel("Platform").selectOption("linux");
  await expect(page).toHaveURL(/platform=linux/);
  await expectDeviceCount(page, expected.linux);

  await page.getByLabel("Control status").selectOption("disconnected");
  await expect(page).toHaveURL(/status=disconnected/);
  await expectDeviceCount(page, expected.disconnected);

  await page.getByLabel("Find device").fill("catalog-node-016");
  await expect(page).toHaveURL(/q=catalog-node-016/);
  await expectDeviceCount(page, expected.named);
});

test("uses a full-screen mobile detail and preserves browser history", async ({
  page,
}, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("mobile"));
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/devices");
  await expect(
    page.getByLabel("Device directory", { exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Device detail", { exact: true })).toBeHidden();

  await page.getByRole("row", { name: /Open catalog-node-001/ }).click();
  await expect(page).toHaveURL(/\/devices\/n_/);
  await expect(page.getByLabel("Device detail", { exact: true })).toBeVisible();
  await expect(
    page.getByLabel("Device directory", { exact: true }),
  ).toBeHidden();
  await expect(
    page.getByText("Directory presence is not traffic activity."),
  ).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("devices-mobile-detail.png"),
    fullPage: true,
  });

  await page.getByRole("button", { name: "Back to devices" }).click();
  await expect(
    page.getByLabel("Device directory", { exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("devices-mobile-list.png"),
    fullPage: true,
  });
});

test("fits the directory controls and rows at 320px", async ({
  page,
}, testInfo) => {
  test.skip(!testInfo.project.name.startsWith("mobile"));
  await page.setViewportSize({ width: 320, height: 700 });
  await page.goto("/devices");
  await expect(
    page.getByRole("row", { name: /Open catalog-node-001/ }),
  ).toBeVisible();

  const geometry = await page.locator(".devices-shell").evaluate((shell) => {
    const overflowing = [
      ...shell.querySelectorAll<HTMLElement>("button, input, select"),
    ]
      .filter((element) => element.offsetParent !== null)
      .filter((element) => element.scrollWidth > element.clientWidth + 1)
      .map(
        (element) =>
          element.getAttribute("aria-label") ?? element.textContent?.trim(),
      );
    return {
      shellWidth: shell.getBoundingClientRect().width,
      scrollWidth: shell.scrollWidth,
      overflowing,
    };
  });
  expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.shellWidth + 1);
  expect(geometry.overflowing).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath("devices-mobile-320.png"),
    fullPage: true,
  });
});

test("shows explicit disabled, stale, empty, and request-error states", async ({
  page,
}) => {
  await page.route("**/api/v1/capabilities", (route) =>
    route.fulfill({ json: { protocolVersions: [1], features: [] } }),
  );
  await page.goto("/devices");
  await expect(
    page.getByText("Device directory is not configured"),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "Devices" })).toBeHidden();

  await page.unroute("**/api/v1/capabilities");
  await page.route("**/api/v1/capabilities", (route) =>
    route.fulfill({
      json: { protocolVersions: [1], features: ["device-directory"] },
    }),
  );
  await page.route("**/api/v1/devices", (route) =>
    route.fulfill({
      json: {
        sync: {
          status: "stale",
          lastSuccessAt: "2026-08-31T10:00:00Z",
          errorCode: "rate-limited",
        },
        devices: [],
      },
    }),
  );
  await page.reload();
  await expect(page.getByText("Directory stale")).toBeVisible();
  await expect(page.getByText("No matching devices")).toBeVisible();

  await page.unroute("**/api/v1/devices");
  await page.route("**/api/v1/devices", (route) =>
    route.fulfill({ status: 503, body: "unavailable" }),
  );
  await page.reload();
  await expect(page.getByText("Device directory unavailable")).toBeVisible();
  await expect(page.getByRole("button", { name: "Retry" })).toBeVisible();
});

function captureBrowserErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  page.on("pageerror", (error) => errors.push(error.message));
  return errors;
}

async function expectDeviceCount(page: Page, count: number) {
  await expect(page.locator(".devices-summary")).toContainText(
    new RegExp(`${count}\\s*device`),
  );
}
