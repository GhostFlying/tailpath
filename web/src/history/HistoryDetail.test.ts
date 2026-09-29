import { describe, expect, it } from "vitest";
import type { EdgeHistory } from "../api/types";
import { summarizeLastPath } from "./HistoryDetail";

describe("summarizeLastPath", () => {
  it("keeps a missing reverse direction explicitly partial", () => {
    const event = {
      observedAt: "2026-09-29T00:00:00Z",
      path: { kind: "direct" },
      conflicts: [],
      observations: [],
      directions: [
        {
          fromNodeId: "a",
          toNodeId: "b",
          primaryPath: { kind: "direct" },
          evidence: "observed",
          observerId: "a",
          collectedAt: "2026-09-29T00:00:00Z",
          receivedAt: "2026-09-29T00:00:01Z",
          clockSkewed: false,
        },
      ],
    } as EdgeHistory["pathEvents"][number];

    expect(summarizeLastPath(event)).toEqual({
      label: "Partial path evidence",
      asymmetric: false,
      partial: true,
    });
  });

  it("keeps different VNIs on the same relay asymmetric", () => {
    const direction = (fromNodeId: string, toNodeId: string, vni: number) => ({
      fromNodeId,
      toNodeId,
      primaryPath: {
        kind: "peer_relay" as const,
        peerRelayStableNodeId: "relay-hz",
        peerRelayEndpoint: "203.0.113.8:41641",
        peerRelayVni: vni,
        peerRelayResolution: "endpoint_match" as const,
      },
      evidence: "observed" as const,
      observerId: fromNodeId,
      collectedAt: "2026-09-29T00:00:00Z",
      receivedAt: "2026-09-29T00:00:01Z",
      clockSkewed: false,
    });
    const event = {
      observedAt: "2026-09-29T00:00:00Z",
      path: { kind: "peer_relay" },
      conflicts: [],
      observations: [],
      directions: [direction("a", "b", 4293), direction("b", "a", 8)],
    } as EdgeHistory["pathEvents"][number];

    expect(summarizeLastPath(event)).toEqual({
      label: "Asymmetric paths",
      asymmetric: true,
      partial: false,
    });
  });
});
