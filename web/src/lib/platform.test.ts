import { describe, expect, it } from "vitest";
import { platformPresentation } from "./platform";

describe("platformPresentation", () => {
  it.each([
    ["linux", "Linux", "/device-linux.svg"],
    ["macos", "macOS", "/device-macos.svg"],
    ["windows", "Windows", "/device-windows.svg"],
    ["ios", "iOS", "/device-ios.svg"],
    ["android", "Android", "/device-android.svg"],
  ])("maps %s to one shared presentation", (os, label, asset) => {
    const presentation = platformPresentation(os);
    expect(presentation.label).toBe(label);
    expect(presentation.asset).toBe(asset);
  });

  it("preserves an unknown reported value for display", () => {
    const presentation = platformPresentation("freebsd-14");
    expect(presentation.label).toBe("freebsd-14");
    expect(presentation.asset).toBe("/device-unknown.svg");
  });
});
