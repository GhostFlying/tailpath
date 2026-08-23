import type {
  EdgeHistory,
  HistoryEdgePage,
  HistoryNodes,
  HistoryWindow,
  PathKind,
  Topology,
} from "./types";

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

export function getHistoryNodes(
  window: HistoryWindow,
  signal?: AbortSignal,
): Promise<HistoryNodes> {
  return getJSON<HistoryNodes>(
    `/api/v1/history/nodes?window=${window}`,
    signal,
  );
}

export interface HistoryEdgeRequest {
  window: HistoryWindow;
  nodeId?: string;
  path?: PathKind;
  cursor?: string;
  limit?: number;
}

export function getHistoryEdges(
  request: HistoryEdgeRequest,
  signal?: AbortSignal,
): Promise<HistoryEdgePage> {
  const query = new URLSearchParams({ window: request.window });
  if (request.nodeId) query.set("nodeId", request.nodeId);
  if (request.path) query.set("path", request.path);
  if (request.cursor) query.set("cursor", request.cursor);
  query.set("limit", String(request.limit ?? 50));
  return getJSON<HistoryEdgePage>(
    `/api/v1/history/edges?${query.toString()}`,
    signal,
  );
}
