import { describe, expect, it } from "vitest";
import type { DirectionalPathState } from "../api/types";
import { directionFallbackLabel } from "./PathTimeline";

const observedDirection: DirectionalPathState = {
  fromNodeId: "a",
  toNodeId: "b",
  primaryPath: { kind: "direct" },
  evidence: "observed",
  observerId: "a",
  collectedAt: "2026-09-29T00:00:00Z",
  receivedAt: "2026-09-29T00:00:01Z",
  clockSkewed: false,
};

describe("directionFallbackLabel", () => {
  it("keeps a missing direction unknown", () => {
    expect(directionFallbackLabel(undefined)).toBe("Unknown");
  });

  it("reserves none for an observed direction without fallback", () => {
    expect(directionFallbackLabel(observedDirection)).toBe("None");
    expect(
      directionFallbackLabel({
        ...observedDirection,
        fallbackPath: { kind: "derp", derpRegion: "hkg" },
      }),
    ).toBe("DERP hkg");
  });
});
