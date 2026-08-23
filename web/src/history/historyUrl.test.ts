import { describe, expect, it } from "vitest";
import {
  defaultHistoryWindow,
  historySearchString,
  parseHistoryURL,
  updateHistoryURL,
} from "./historyUrl";

describe("history URL state", () => {
  it("normalizes invalid values without forwarding them to the API", () => {
    expect(
      parseHistoryURL(
        new URLSearchParams("window=forever&path=magic&nodeId=node-a"),
      ),
    ).toEqual({
      window: defaultHistoryWindow,
      path: "",
      nodeId: "node-a",
      cursor: "",
    });
  });

  it("clears pagination whenever a filter changes", () => {
    const next = updateHistoryURL(
      new URLSearchParams("window=1h&cursor=next&nodeId=a"),
      { path: "derp" },
    );
    expect(next.get("cursor")).toBeNull();
    expect(next.get("path")).toBe("derp");
  });

  it("omits default and empty values from shareable links", () => {
    expect(
      historySearchString({
        window: "24h",
        nodeId: "",
        path: "",
        cursor: "",
      }),
    ).toBe("");
  });
});
