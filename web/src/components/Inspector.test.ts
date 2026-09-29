import { describe, expect, it } from "vitest";
import type { DirectionalPathState } from "../api/types";
import { fallbackExplanation } from "./Inspector";

function direction(evidence: "observed" | "inferred"): DirectionalPathState {
  return {
    fromNodeId: "a",
    toNodeId: "b",
    primaryPath: { kind: "peer_relay", peerRelayVni: 8 },
    fallbackPath: { kind: "derp", derpRegion: "hkg" },
    evidence,
    inferenceRule:
      evidence === "inferred" ? "tailscale-status-fallback-v1" : undefined,
    observerId: "a",
    collectedAt: "2026-09-29T00:00:00Z",
    receivedAt: "2026-09-29T00:00:01Z",
    clockSkewed: false,
  };
}

describe("fallbackExplanation", () => {
  it("does not describe observed fallback evidence as inferred", () => {
    expect(fallbackExplanation([direction("observed")])).toContain(
      "observed parallel route",
    );
    expect(fallbackExplanation([direction("observed")])).not.toContain(
      "an inferred parallel route",
    );
  });

  it("describes inferred and mixed evidence explicitly", () => {
    expect(fallbackExplanation([direction("inferred")])).toContain(
      "an inferred parallel route",
    );
    expect(
      fallbackExplanation([direction("observed"), direction("inferred")]),
    ).toContain("observed and inferred parallel routes");
  });
});
