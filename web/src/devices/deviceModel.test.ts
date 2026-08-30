import { describe, expect, it, vi } from "vitest";
import type { DirectoryDevice } from "../api/types";
import {
  controlStatus,
  deviceLabel,
  deviceSearchText,
  formatDeviceAge,
  runtimeStatus,
} from "./deviceModel";

describe("deviceModel", () => {
  it("uses the short MagicDNS label and indexes all searchable metadata", () => {
    const device = fixtureDevice();
    expect(deviceLabel(device)).toBe("build-host");
    expect(deviceSearchText(device)).toContain("build-host.example.ts.net");
    expect(deviceSearchText(device)).toContain("fd7a:115c:a1e0::1");
    expect(deviceSearchText(device)).toContain("tag:ci");
  });

  it("keeps control and runtime status independent", () => {
    const directoryOnly = fixtureDevice({ runtime: undefined });
    expect(controlStatus(directoryOnly)).toBe("Connected");
    expect(runtimeStatus(directoryOnly)).toBe("Runtime unobserved");

    const stale = fixtureDevice({
      connectedToControl: false,
      runtime: runtimeEvidence(false),
    });
    expect(controlStatus(stale)).toBe("Disconnected");
    expect(runtimeStatus(stale)).toBe("Runtime stale");
  });

  it("formats bounded relative ages", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-31T12:00:00Z"));
    expect(formatDeviceAge("2026-08-31T11:58:00Z")).toBe("2m ago");
    expect(formatDeviceAge(undefined)).toBe("Never");
    vi.useRealTimers();
  });
});

function fixtureDevice(
  overrides: Partial<DirectoryDevice> = {},
): DirectoryDevice {
  return {
    id: "node-1",
    stableNodeId: "stable-1",
    dnsName: "build-host.example.ts.net.",
    hostname: "build-host",
    platform: "linux",
    tailscaleIps: ["100.64.0.1", "fd7a:115c:a1e0::1"],
    tags: ["tag:ci"],
    connectedToControl: true,
    collectedAt: "2026-08-31T12:00:00Z",
    identityStatus: "resolved",
    runtime: runtimeEvidence(true),
    conflicts: [],
    ...overrides,
  };
}

function runtimeEvidence(
  online: boolean,
): NonNullable<DirectoryDevice["runtime"]> {
  return {
    tailscaleIps: ["100.64.0.1"],
    observable: true,
    online,
    lastEvidenceAt: "2026-08-31T11:59:00Z",
    collectedAt: "2026-08-31T12:00:00Z",
  };
}
