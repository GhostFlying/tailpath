import type { HistoryWindow, PathKind } from "../api/types";

export const defaultHistoryWindow: HistoryWindow = "24h";

const historyWindows = new Set<HistoryWindow>(["15m", "1h", "6h", "24h", "7d"]);
const pathKinds = new Set<PathKind>([
  "direct",
  "derp",
  "peer_relay",
  "unknown",
]);

export interface HistoryURLState {
  window: HistoryWindow;
  nodeId: string;
  path: PathKind | "";
  cursor: string;
}

export function parseHistoryURL(search: URLSearchParams): HistoryURLState {
  const rawWindow = search.get("window") as HistoryWindow | null;
  const rawPath = search.get("path") as PathKind | null;
  return {
    window:
      rawWindow && historyWindows.has(rawWindow)
        ? rawWindow
        : defaultHistoryWindow,
    nodeId: boundedValue(search.get("nodeId")),
    path: rawPath && pathKinds.has(rawPath) ? rawPath : "",
    cursor: boundedValue(search.get("cursor"), 2048),
  };
}

export function updateHistoryURL(
  current: URLSearchParams,
  update: Partial<HistoryURLState>,
): URLSearchParams {
  const next = new URLSearchParams(current);
  for (const [key, value] of Object.entries(update)) {
    if (!value || (key === "window" && value === defaultHistoryWindow)) {
      next.delete(key);
    } else {
      next.set(key, value);
    }
  }
  if (
    !("cursor" in update) &&
    ("window" in update || "nodeId" in update || "path" in update)
  ) {
    next.delete("cursor");
  }
  return next;
}

export function historySearchString(state: HistoryURLState): string {
  return updateHistoryURL(new URLSearchParams(), state).toString();
}

function boundedValue(value: string | null, maxLength = 512): string {
  return value && value.length <= maxLength ? value : "";
}
