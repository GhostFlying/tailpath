import { describe, expect, it } from "vitest";
import type { TopologyEdge } from "../api/types";
import {
  directionCoverage,
  inboundPathLimit,
  liveTrafficObservation,
  unknownPathReason,
} from "./directionEvidence";

const at = "2026-10-02T08:37:10Z";
const receiver: TopologyEdge["observations"][number] = {
  observerId: "smallbox",
  path: { kind: "derp", derpRegion: "hgh-custom" },
  collectedAt: at,
  receivedAt: at,
  clockSkewed: false,
};

describe("directional observation coverage", () => {
  it("explains a mobile sender's missing path without mirroring the receiver", () => {
    const coverage = directionCoverage("iphone", "smallbox", [receiver]);
    expect(coverage.point).toBe("receiver");
    expect(unknownPathReason(coverage)).toContain(inboundPathLimit);
    expect(unknownPathReason(coverage, true)).toBe(
      "No sender path observation retained.",
    );
    expect(directionCoverage("smallbox", "iphone", [receiver]).point).toBe(
      "sender",
    );
  });

  it("does not turn relay evidence into an endpoint report", () => {
    const relay = {
      ...receiver,
      relaySession: {
        sessionId: "relay-session",
        vni: 8,
        sourceIdentityStatus: "resolved" as const,
        targetIdentityStatus: "resolved" as const,
      },
    };
    expect(directionCoverage("iphone", "smallbox", [relay]).point).toBe("none");
    expect(
      unknownPathReason(directionCoverage("iphone", "smallbox", []), true),
    ).toBe("No path observation retained for this direction.");
  });

  it("distinguishes an explicit unknown sender path from receiver-only coverage", () => {
    const sender = {
      ...receiver,
      observerId: "iphone",
      path: { kind: "unknown" as const },
    };
    const coverage = directionCoverage("iphone", "smallbox", [
      receiver,
      sender,
    ]);
    expect(coverage.point).toBe("sender");
    expect(unknownPathReason(coverage)).toBe(
      "The sender reported no usable path.",
    );
  });
});

describe("live traffic provenance", () => {
  it("uses the newest relay counters only when no endpoint report is current", () => {
    const relay = {
      ...receiver,
      observerId: "relay",
      path: { kind: "peer_relay" as const },
    };
    const older = {
      ...relay,
      observerId: "older-relay",
      receivedAt: "2026-10-02T08:37:09Z",
    };
    expect(
      liveTrafficObservation("iphone", "smallbox", [older, relay], at),
    ).toEqual({ point: "relay", observation: relay });
    expect(
      liveTrafficObservation("iphone", "smallbox", [relay, receiver], at)
        ?.point,
    ).toBe("receiver");
  });

  it("uses receiver RX counters when the mobile sender does not report", () => {
    expect(
      liveTrafficObservation("iphone", "smallbox", [receiver], at)?.point,
    ).toBe("receiver");
    expect(
      liveTrafficObservation("smallbox", "iphone", [receiver], at)?.point,
    ).toBe("sender");
  });

  it("prefers current sender TX counters and falls back to current receiver RX", () => {
    const sender = { ...receiver, observerId: "iphone" };
    expect(
      liveTrafficObservation("iphone", "smallbox", [receiver, sender], at)
        ?.observation,
    ).toBe(sender);
    const expired = { ...sender, receivedAt: "2026-10-02T08:36:59Z" };
    expect(
      liveTrafficObservation("iphone", "smallbox", [receiver, expired], at)
        ?.observation,
    ).toBe(receiver);
  });

  it("does not attribute live traffic to an old report or a skewed collection time", () => {
    const skewed = {
      ...receiver,
      collectedAt: "2027-01-01T00:00:00Z",
      clockSkewed: true,
    };
    expect(
      liveTrafficObservation("iphone", "smallbox", [skewed], at)?.point,
    ).toBe("receiver");
    expect(
      liveTrafficObservation(
        "iphone",
        "smallbox",
        [receiver],
        "2026-10-02T08:37:20Z",
      ),
    ).not.toBeNull();
    expect(
      liveTrafficObservation(
        "iphone",
        "smallbox",
        [receiver],
        "2026-10-02T08:37:20.001Z",
      ),
    ).toBeNull();
  });
});
