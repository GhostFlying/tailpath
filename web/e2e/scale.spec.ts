import { expect, test } from "@playwright/test";

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
  await testInfo.attach("scale-browser.json", {
    body: JSON.stringify(
      {
        project: testInfo.project.name,
        readyElapsedMs,
        topologyNodes: topology.nodes.length,
        logicalEdges: topology.edges.length,
        renderedNodes: 505,
        consoleErrors,
      },
      null,
      2,
    ),
    contentType: "application/json",
  });
  await page.screenshot({
    path: testInfo.outputPath("tailpath-scale.png"),
    fullPage: true,
  });
});
