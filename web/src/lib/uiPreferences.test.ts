import { describe, expect, it } from "vitest";
import {
  readShowRecentPreference,
  showRecentPreferenceKey,
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
