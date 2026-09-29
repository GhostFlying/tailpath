import type {
  EdgeHistory,
  DeviceDirectory,
  HistoryEdgePage,
  HistoryNodes,
  HistoryWindow,
  PathEventPage,
  PathKind,
  Topology,
  ServerCapabilities,
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

export function getCapabilities(
  signal?: AbortSignal,
): Promise<ServerCapabilities> {
  return getJSON<ServerCapabilities>("/api/v1/capabilities", signal);
}

export function getDevices(signal?: AbortSignal): Promise<DeviceDirectory> {
  return getJSON<DeviceDirectory>("/api/v1/devices", signal).then(
    (directory) => ({ ...directory, devices: directory.devices ?? [] }),
  );
}

export function getEdgeHistory(
  edgeId: string,
  signal?: AbortSignal,
  window: HistoryWindow = "1h",
): Promise<EdgeHistory> {
  return getJSON<NullableEdgeHistory>(
    `/api/v1/history/edges/${encodeURIComponent(edgeId)}?window=${window}`,
    signal,
  ).then(normalizeEdgeHistory);
}

type NullablePathEvent = Omit<
  EdgeHistory["pathEvents"][number],
  "conflicts" | "observations" | "directions"
> & {
  conflicts: EdgeHistory["pathEvents"][number]["conflicts"] | null;
  observations: EdgeHistory["pathEvents"][number]["observations"] | null;
  directions?: EdgeHistory["pathEvents"][number]["directions"] | null;
};

type NullableEdgeHistory = Omit<
  EdgeHistory,
  "relatedNodes" | "traffic" | "pathAnchor" | "pathEvents"
> & {
  relatedNodes: EdgeHistory["relatedNodes"] | null;
  traffic: EdgeHistory["traffic"] | null;
  pathAnchor?: NullablePathEvent | null;
  pathEvents: NullablePathEvent[] | null;
};

function normalizePathEvent(event: NullablePathEvent) {
  const { directions, ...rest } = event;
  return {
    ...rest,
    conflicts: event.conflicts ?? [],
    observations: event.observations ?? [],
    // Presence is meaningful: a current server emits an explicit empty array
    // when directional evidence was withdrawn, while older servers omit (or
    // return null for) the field entirely.
    ...(directions == null ? {} : { directions }),
  };
}

type NullablePathEventPage = Omit<
  PathEventPage,
  "anchor" | "events" | "relatedNodes"
> & {
  anchor?: NullablePathEvent | null;
  events: NullablePathEvent[] | null;
  relatedNodes: PathEventPage["relatedNodes"] | null;
};

export function getEdgePathHistory(
  edgeId: string,
  window: HistoryWindow,
  cursor = "",
  signal?: AbortSignal,
  limit = 500,
  to?: string,
): Promise<PathEventPage> {
  const query = new URLSearchParams({ window, limit: String(limit) });
  if (to) query.set("to", to);
  if (cursor) query.set("cursor", cursor);
  return getJSON<NullablePathEventPage>(
    `/api/v1/history/edges/${encodeURIComponent(edgeId)}/paths?${query.toString()}`,
    signal,
  ).then((page) => ({
    ...page,
    anchor: page.anchor ? normalizePathEvent(page.anchor) : undefined,
    events: (page.events ?? []).map(normalizePathEvent),
    relatedNodes: page.relatedNodes ?? [],
  }));
}

function normalizeEdgeHistory(history: NullableEdgeHistory): EdgeHistory {
  return {
    ...history,
    relatedNodes: history.relatedNodes ?? [],
    traffic: history.traffic ?? [],
    pathAnchor: history.pathAnchor
      ? normalizePathEvent(history.pathAnchor)
      : undefined,
    pathEvents: (history.pathEvents ?? []).map(normalizePathEvent),
  };
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
