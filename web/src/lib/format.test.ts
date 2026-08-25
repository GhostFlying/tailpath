import { describe, expect, it } from "vitest";
import {
  formatCompactRate,
  formatRate,
  nodeLabel,
  pathLabel,
  runtimeReportingLabel,
} from "./format";

describe("formatRate", () => {
  it("uses compact decimal units", () => {
    expect(formatRate(0)).toBe("0 B/s");
    expect(formatRate(1530)).toBe("1.53 KB/s");
    expect(formatRate(12500)).toBe("12.5 KB/s");
  });
});

describe("formatCompactRate", () => {
  it("limits graph labels to one decimal place", () => {
    expect(formatCompactRate(530)).toBe("530 B/s");
    expect(formatCompactRate(2000)).toBe("2.0 KB/s");
    expect(formatCompactRate(12_500)).toBe("13 KB/s");
  });
});

describe("runtimeReportingLabel", () => {
  it("does not imply an expected total when every known runtime reports", () => {
    expect(runtimeReportingLabel(0, 0)).toBe("0 runtimes reporting");
    expect(runtimeReportingLabel(1, 0)).toBe("1 runtime reporting");
    expect(runtimeReportingLabel(2, 0)).toBe("2 runtimes reporting");
  });

  it("separates reporting and stale known runtimes", () => {
    expect(runtimeReportingLabel(2, 1)).toBe("2 reporting · 1 stale");
  });
});

describe("pathLabel", () => {
  it("includes a DERP region when known", () => {
    expect(pathLabel({ kind: "derp", derpRegion: "hkg" })).toBe("DERP hkg");
  });
});

describe("nodeLabel", () => {
  it("prefers the MagicDNS short name over hostname", () => {
    expect(
      nodeLabel({
        id: "n_1",
        stableNodeId: "stable-1",
        hostname: "host-device",
        dnsName: "friendly-name.example.ts.net.",
        observable: true,
        online: true,
        lastEvidenceAt: "2026-08-23T00:00:00Z",
        clockSkewed: false,
      }),
    ).toBe("friendly-name");
  });
});
