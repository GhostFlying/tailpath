import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";

test.skip(
  process.env.TAILPATH_RELAY_SCALE_E2E !== "1",
  "the 1,000-session relay browser gate is workflow_dispatch only",
);

test("renders 1,000 sanitized third-party relay sessions", async ({
  page,
}, testInfo) => {
  test.setTimeout(180_000);
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });

  const startedAt = Date.now();
  await page.goto("/");
  const graph = page.getByLabel("Live Tailnet topology");
  await expect(graph).toHaveAttribute("data-edge-count", "1000", {
    timeout: 120_000,
  });
  await expect(graph).toHaveAttribute("data-node-count", "250");
  await expect(graph).toHaveAttribute("data-device-node-count", "242");
  await expect(graph).toHaveAttribute("data-ready", "true", {
    timeout: 120_000,
  });
  const readyElapsedMs = Date.now() - startedAt;
  if (testInfo.project.name === "desktop-chromium") {
    expect(readyElapsedMs).toBeLessThanOrEqual(5_000);
  }
  await expect(graph).toHaveAttribute("data-layout-runs", "1");
  const firstPositions = await graph.getAttribute("data-layout-positions");

  const evidence = await page.evaluate(async () => {
    const topologyResponse = await fetch("/api/v1/topology");
    const topologyText = await topologyResponse.text();
    const topology = JSON.parse(topologyText) as {
      nodes: unknown[];
      edges: Array<{
        id: string;
        path: { kind: string; peerRelayVni?: number };
        observations: Array<{ relaySession?: { sessionId: string } }>;
      }>;
      observers: unknown[];
    };
    const listResponse = await fetch(
      "/api/v1/history/edges?window=1h&path=peer_relay&limit=1",
    );
    const list = (await listResponse.json()) as {
      edges: Array<{ edgeId: string }>;
    };
    const detailResponse = await fetch(
      `/api/v1/history/edges/${encodeURIComponent(list.edges[0].edgeId)}?window=1h`,
    );
    const detailText = await detailResponse.text();
    return { topology, topologyText, detailText };
  });
  expect(evidence.topology.nodes).toHaveLength(250);
  expect(evidence.topology.edges).toHaveLength(1000);
  expect(evidence.topology.observers).toHaveLength(8);
  expect(
    evidence.topology.edges.every(
      (edge) =>
        edge.path.kind === "peer_relay" &&
        edge.path.peerRelayVni !== undefined &&
        edge.observations.length === 1 &&
        edge.observations[0].relaySession?.sessionId,
    ),
  ).toBe(true);
  for (const payload of [evidence.topologyText, evidence.detailText]) {
    expect(payload).not.toContain("192.0.2.");
    expect(payload).not.toContain("2001:db8::");
    expect(payload).not.toContain("d:feed");
    expect(payload).not.toContain('"endpoint"');
  }
  expect(evidence.detailText).toContain('"peer_relay"');
  expect(evidence.detailText).toContain('"relaySession"');
  expect(consoleErrors).toEqual([]);

  const reloadStartedAt = Date.now();
  await page.reload();
  await expect(graph).toHaveAttribute("data-ready", "true", {
    timeout: 30_000,
  });
  const cachedReadyElapsedMs = Date.now() - reloadStartedAt;
  await expect(graph).toHaveAttribute("data-layout-runs", "0");
  await expect(graph).toHaveAttribute(
    "data-layout-positions",
    firstPositions ?? "",
  );

  const metrics = JSON.stringify(
    {
      project: testInfo.project.name,
      readyElapsedMs,
      cachedReadyElapsedMs,
      canonicalNodes: evidence.topology.nodes.length,
      relayObservers: evidence.topology.observers.length,
      relaySessions: evidence.topology.edges.length,
      renderedNodes: 250,
      consoleErrors,
    },
    null,
    2,
  );
  await writeFile(testInfo.outputPath("relay-scale-browser.json"), metrics);
  await testInfo.attach("relay-scale-browser.json", {
    body: metrics,
    contentType: "application/json",
  });
  await page.screenshot({
    path: testInfo.outputPath("tailpath-relay-scale.png"),
    fullPage: true,
  });
});
