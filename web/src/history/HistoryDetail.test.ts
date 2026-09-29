import { describe, expect, it } from "vitest";
import type { EdgeHistory } from "../api/types";
import { summarizeLastPath } from "./HistoryDetail";

describe("summarizeLastPath", () => {
  it("keeps a missing reverse direction explicitly partial", () => {
    const event = {
      observedAt: "2026-09-29T00:00:00Z",
      path: { kind: "direct" },
      conflicts: [],
      observations: [],
      directions: [
        {
          fromNodeId: "a",
          toNodeId: "b",
          primaryPath: { kind: "direct" },
          evidence: "observed",
          observerId: "a",
          collectedAt: "2026-09-29T00:00:00Z",
          receivedAt: "2026-09-29T00:00:01Z",
          clockSkewed: false,
        },
      ],
    } as EdgeHistory["pathEvents"][number];

    expect(summarizeLastPath(event)).toEqual({
      label: "Partial path evidence",
      asymmetric: false,
      partial: true,
    });
  });
});
