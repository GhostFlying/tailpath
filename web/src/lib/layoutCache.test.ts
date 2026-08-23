import { describe, expect, it } from "vitest";
import {
  clearLayoutCache,
  layoutCacheKey,
  layoutCacheMaxAgeMs,
  layoutCacheMaxEntries,
  readLayoutCache,
  writeLayoutCache,
} from "./layoutCache";

function memoryStorage(initial?: string): Storage {
  const values = new Map<string, string>();
  if (initial !== undefined) values.set(layoutCacheKey, initial);
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
    removeItem: (key: string) => values.delete(key),
  } as unknown as Storage;
}

describe("layout cache", () => {
  it("round-trips finite canonical positions and refreshes last seen", () => {
    const storage = memoryStorage();
    writeLayoutCache(
      storage,
      new Map([["node:a", { x: 12.5, y: -8, lastSeen: 1 }]]),
      ["node:a"],
      1_000,
    );
    expect(readLayoutCache(storage, 1_000).get("node:a")).toEqual({
      x: 12.5,
      y: -8,
      lastSeen: 1_000,
    });
  });

  it.each(["not-json", "{}", '{"version":2,"nodes":[]}'])(
    "ignores malformed payload %s",
    (payload) => {
      expect(readLayoutCache(memoryStorage(payload))).toEqual(new Map());
    },
  );

  it("rejects an oversized entry collection", () => {
    const nodes = Array.from(
      { length: layoutCacheMaxEntries + 1 },
      (_, index) => ({
        id: `node:${index}`,
        x: index,
        y: index,
        lastSeen: 1,
      }),
    );
    const storage = memoryStorage(JSON.stringify({ version: 1, nodes }));
    expect(readLayoutCache(storage, 1)).toEqual(new Map());
  });

  it("drops stale and invalid entries independently", () => {
    const now = layoutCacheMaxAgeMs + 10_000;
    const storage = memoryStorage(
      JSON.stringify({
        version: 1,
        nodes: [
          { id: "fresh", x: 1, y: 2, lastSeen: now },
          { id: "stale", x: 1, y: 2, lastSeen: 1 },
          { id: "infinite", x: null, y: 2, lastSeen: now },
          { id: "", x: 1, y: 2, lastSeen: now },
        ],
      }),
    );
    expect([...readLayoutCache(storage, now).keys()]).toEqual(["fresh"]);
  });

  it("keeps only the newest two thousand positions", () => {
    const storage = memoryStorage();
    const positions = new Map(
      Array.from(
        { length: layoutCacheMaxEntries + 20 },
        (_, index) =>
          [
            `node:${index}`,
            { x: index, y: index, lastSeen: index + 1 },
          ] as const,
      ),
    );
    writeLayoutCache(storage, positions, [], layoutCacheMaxAgeMs);
    const result = readLayoutCache(storage, layoutCacheMaxAgeMs);
    expect(result).toHaveLength(layoutCacheMaxEntries);
    expect(result.has("node:0")).toBe(false);
    expect(result.has(`node:${layoutCacheMaxEntries + 19}`)).toBe(true);
  });

  it("survives blocked storage and clears when available", () => {
    const blocked = {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
      removeItem: () => {
        throw new Error("blocked");
      },
    } as unknown as Storage;
    expect(readLayoutCache(blocked)).toEqual(new Map());
    expect(() => writeLayoutCache(blocked, new Map(), [])).not.toThrow();
    expect(() => clearLayoutCache(blocked)).not.toThrow();

    const storage = memoryStorage();
    writeLayoutCache(
      storage,
      new Map([["a", { x: 1, y: 2, lastSeen: 3 }]]),
      [],
      3,
    );
    clearLayoutCache(storage);
    expect(readLayoutCache(storage, 3)).toEqual(new Map());
  });
});
