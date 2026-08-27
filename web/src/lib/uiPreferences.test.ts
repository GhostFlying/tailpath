import { describe, expect, it } from "vitest";
import {
  readShowRecentPreference,
  readShowControlTrafficPreference,
  showControlTrafficPreferenceKey,
  showRecentPreferenceKey,
  writeShowControlTrafficPreference,
  writeShowRecentPreference,
} from "./uiPreferences";

describe("show recent preference", () => {
  it("defaults on and persists explicit choices", () => {
    const values = new Map<string, string>();
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
    } as unknown as Storage;

    expect(readShowRecentPreference(storage)).toBe(true);
    writeShowRecentPreference(storage, false);
    expect(values.get(showRecentPreferenceKey)).toBe("false");
    expect(readShowRecentPreference(storage)).toBe(false);
  });

  it("falls back to on when storage access fails", () => {
    const storage = {
      getItem: () => {
        throw new Error("blocked");
      },
    } as unknown as Storage;
    expect(readShowRecentPreference(storage)).toBe(true);
  });
});

describe("control traffic preference", () => {
  it("defaults off and persists explicit choices", () => {
    const values = new Map<string, string>();
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
    } as unknown as Storage;

    expect(readShowControlTrafficPreference(storage)).toBe(false);
    writeShowControlTrafficPreference(storage, true);
    expect(values.get(showControlTrafficPreferenceKey)).toBe("true");
    expect(readShowControlTrafficPreference(storage)).toBe(true);
  });
});
