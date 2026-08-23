import type { EdgeHistory, HistoryWindow, Topology } from "./types";

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {
    headers: { Accept: "application/json" },
    signal,
  });
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }
  return (await response.json()) as T;
}

export function getTopology(signal?: AbortSignal): Promise<Topology> {
  return getJSON<Topology>("/api/v1/topology", signal);
}

export function getEdgeHistory(
  edgeId: string,
  signal?: AbortSignal,
  window: HistoryWindow = "1h",
): Promise<EdgeHistory> {
  return getJSON<EdgeHistory>(
    `/api/v1/history/edges/${encodeURIComponent(edgeId)}?window=${window}`,
    signal,
  );
}
