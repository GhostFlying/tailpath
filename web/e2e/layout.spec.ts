import { expect, test, type Page } from "@playwright/test";

test.skip(
  process.env.TAILPATH_LAYOUT_E2E !== "1",
  "requires test-only geometry diagnostics",
);
const positions = [
  ["phone", -220, -240],
  ["peer", -260, 0],
  ["smallbox", 0, 0],
  ["desktop", 280, -60],
  ["devbox", 230, 150],
  ["home", 180, 370],
] as const;
const names = [
  "pixel-6-pro",
  "dd0b7984e35f8268",
  "smallbox",
  "desktop",
  "devbox",
  "home-singbox",
];
interface Rect {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}
interface Geometry {
  width: number;
  height: number;
  bodies: { id: string; bounds: Rect }[];
  labels: {
    id: string;
    owner: string;
    kind: string;
    internal: boolean;
    bounds: Rect;
  }[];
  mode: string;
  hidden: number;
}
function overlap(a: Rect, b: Rect, gap = 0) {
  return (
    a.x1 < b.x2 + gap &&
    a.x2 > b.x1 - gap &&
    a.y1 < b.y2 + gap &&
    a.y2 > b.y1 - gap
  );
}
async function fixture(
  page: Page,
  cached = true,
  labels = names,
  traffic = { rate: 13312, revision: 0 },
) {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  if (cached)
    await page.addInitScript(
      (entries) =>
        localStorage.setItem(
          "tailpath.graph-layout.v1",
          JSON.stringify({
            version: 1,
            nodes: entries.map(([id, x, y]) => ({
              id,
              x,
              y,
              lastSeen: Date.now(),
            })),
          }),
        ),
      positions,
    );
  const at = "2026-09-12T00:00:00Z";
  await page.route("**/api/v1/topology", (route) =>
    route.fulfill({
      json: {
        generatedAt: traffic.revision ? "2026-09-12T00:00:02Z" : at,
        nodes: positions.map(([id], i) => ({
          id,
          stableNodeId: id,
          hostname: labels[i],
          os: i < 2 ? "android" : "linux",
          observable: i > 1,
          online: true,
          lastEvidenceAt: at,
          clockSkewed: false,
        })),
        edges: [
          ["phone", "smallbox", "direct"],
          ["peer", "smallbox", "derp"],
          ["smallbox", "desktop", "derp"],
          ["smallbox", "devbox", "derp"],
          ["smallbox", "home", "direct"],
        ].map(([source, target, kind], i) => ({
          id: `edge-${i}`,
          source,
          target,
          path: kind === "derp" ? { kind, derpRegion: "hgh-custom" } : { kind },
          state: "active",
          aToBBytesPerSecond: traffic.rate,
          bToABytesPerSecond: 200,
          lastActive: at,
          observations: [],
        })),
        observers: [],
      },
    }),
  );
  return errors;
}
async function settled(page: Page) {
  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toHaveAttribute("data-ready", "true");
  await expect(graph).toHaveAttribute("data-presentation-ready", "true");
  return JSON.parse((await graph.getAttribute("data-geometry"))!) as Geometry;
}
function clearNames(g: Geometry) {
  for (const object of [...g.bodies, ...g.labels]) {
    expect(
      object.bounds.x1,
      `${object.id} clipped left`,
    ).toBeGreaterThanOrEqual(-1);
    expect(object.bounds.y1, `${object.id} clipped top`).toBeGreaterThanOrEqual(
      -1,
    );
    expect(object.bounds.x2, `${object.id} clipped right`).toBeLessThanOrEqual(
      g.width + 1,
    );
    expect(object.bounds.y2, `${object.id} clipped bottom`).toBeLessThanOrEqual(
      g.height + 1,
    );
  }
  expect(g.labels.filter((l) => l.kind === "name")).toHaveLength(7);
  for (const label of g.labels.filter((l) => !l.internal)) {
    for (const node of g.bodies) {
      if (node.id === label.owner) continue;
      expect(
        overlap(label.bounds, node.bounds, 3),
        `${label.id} covers ${node.id}`,
      ).toBe(false);
    }
  }
  for (let i = 0; i < g.labels.length; i++)
    for (let j = i + 1; j < g.labels.length; j++)
      expect(
        overlap(g.labels[i].bounds, g.labels[j].bounds, 3),
        `${g.labels[i].id} covers ${g.labels[j].id}`,
      ).toBe(false);
}
for (const [width, height] of [
  [320, 568],
  [390, 844],
  [430, 932],
  [844, 390],
  [1440, 900],
]) {
  test(`synthetic shared DERP layout ${width}x${height}`, async ({
    page,
  }, info) => {
    await page.setViewportSize({ width, height });
    const errors = await fixture(page);
    await page.goto("/");
    const g = await settled(page);
    clearNames(g);
    const graph = page.getByLabel("Live Tailnet topology");
    const before = await graph.getAttribute("data-layout-positions");
    await page.getByRole("button", { name: "Fit graph", exact: true }).click();
    await settled(page);
    expect(await graph.getAttribute("data-layout-positions")).toBe(before);
    await page.screenshot({
      path: info.outputPath(`synthetic-layout-${width}.png`),
      fullPage: true,
    });
    await page.getByText("Graph objects", { exact: true }).click();
    await page.getByRole("button", { name: "smallbox", exact: true }).click();
    await expect(page.getByLabel("Topology details")).toContainText("smallbox");
    await page.getByRole("button", { name: "Close details" }).click();
    await page.getByText("Graph objects", { exact: true }).click();
    await page.reload();
    clearNames(await settled(page));
    expect(await graph.getAttribute("data-layout-positions")).toBe(before);
    expect(errors).toEqual([]);
  });
}
test("cold synthetic layout keeps canonical bodies and supports relayout", async ({
  page,
}, info) => {
  await fixture(page, false);
  await page.goto("/");
  clearNames(await settled(page));
  await page
    .getByRole("button", { name: "Relayout graph", exact: true })
    .click();
  clearNames(await settled(page));
  await page.screenshot({
    path: info.outputPath("synthetic-cold-layout.png"),
    fullPage: true,
  });
});

test("long Unicode identities remain inspectable after fonts and rate changes", async ({
  page,
}, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const labels = [
    "生产环境-东京-手机-01",
    "production-same-prefix-device-abcdefghijklmnop-01",
    "production-same-prefix-device-abcdefghijklmnop-02",
    "桌面工作站-日本",
    "开发服务器-上海",
    "家庭网关-长名称",
  ];
  const traffic = { rate: 20, revision: 0 };
  const errors = await fixture(page, true, labels, traffic);
  await page.goto("/");
  await settled(page);
  const graph = page.getByLabel("Live Tailnet topology");
  const positions = await graph.getAttribute("data-layout-positions");
  const viewport = await graph.getAttribute("data-viewport");
  const signature = await graph.getAttribute("data-edge-rate-signature");
  traffic.rate = 123456789;
  traffic.revision++;
  await expect
    .poll(() => graph.getAttribute("data-edge-rate-signature"), {
      timeout: 10000,
    })
    .not.toBe(signature);
  await page.evaluate(() =>
    document.fonts.dispatchEvent(new Event("loadingdone")),
  );
  await settled(page);
  expect(await graph.getAttribute("data-layout-positions")).toBe(positions);
  expect(await graph.getAttribute("data-viewport")).toBe(viewport);
  const list = page.getByText("Graph objects", { exact: true });
  await list.focus();
  await page.keyboard.press("Enter");
  const button = page.getByRole("button", { name: labels[2], exact: true });
  await button.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByLabel("Topology details")).toContainText(labels[2]);
  await page.screenshot({
    path: info.outputPath("synthetic-long-identity-selected.png"),
    fullPage: true,
  });
  expect(errors).toEqual([]);
});
