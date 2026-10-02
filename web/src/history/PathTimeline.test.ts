import { describe, expect, it } from "vitest";
import type { DirectionalPathState } from "../api/types";
import { directionFallbackLabel, formatTimelineTick } from "./PathTimeline";

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
    expect(
      directionFallbackLabel({
        ...observedDirection,
        primaryPath: { kind: "unknown" },
      }),
    ).toBe("Unknown");
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

describe("formatTimelineTick", () => {
  it("distinguishes equal clock times across a multi-day window", () => {
    const from = "2026-09-20T00:00:00Z";
    const to = "2026-09-27T00:00:00Z";

    expect(formatTimelineTick(from, from, to)).not.toBe(
      formatTimelineTick(to, from, to),
    );
  });

  it("keeps same-day ticks compact", () => {
    const from = "2026-09-20T00:00:00Z";
    const to = "2026-09-20T06:00:00Z";

    expect(formatTimelineTick(from, from, to)).toBe(
      new Intl.DateTimeFormat(undefined, {
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      }).format(new Date(from)),
    );
  });
});
