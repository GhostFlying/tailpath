import { useEffect, useState } from "react";
import {
  getEdgeHistory,
  getEdgePathHistory,
  getHistoryEdges,
  getHistoryNodes,
} from "../api/client";
import type { EdgeHistory, HistoryEdgePage, HistoryNodes } from "../api/types";
import type { HistoryURLState } from "./historyUrl";

interface HistoryIndexState {
  nodes: HistoryNodes | null;
  page: HistoryEdgePage | null;
  loading: boolean;
  error: string | null;
  retry: number;
}

export function useHistoryIndex(query: HistoryURLState, enabled = true) {
  const [state, setState] = useState<HistoryIndexState>({
    nodes: null,
    page: null,
    loading: true,
    error: null,
    retry: 0,
  });
  useEffect(() => {
    if (!enabled) {
      setState((current) => ({
        ...current,
        nodes: null,
        page: null,
        loading: false,
        error: null,
      }));
      return;
    }
    const controller = new AbortController();
    setState((current) => ({
      ...current,
      nodes: null,
      page: null,
      loading: true,
      error: null,
    }));
    void Promise.all([
      getHistoryNodes(query.window, controller.signal),
      getHistoryEdges(
        {
          window: query.window,
          nodeId: query.nodeId || undefined,
          path: query.path || undefined,
          cursor: query.cursor || undefined,
        },
        controller.signal,
      ),
    ])
      .then(([nodes, page]) => {
        setState((current) => ({
          ...current,
          nodes,
          page,
          loading: false,
        }));
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setState((current) => ({
          ...current,
          loading: false,
          error: error instanceof Error ? error.message : "History unavailable",
        }));
      });
    return () => controller.abort();
  }, [
    enabled,
    query.cursor,
    query.nodeId,
    query.path,
    query.window,
    state.retry,
  ]);
  return {
    ...state,
    retryRequest: () =>
      setState((current) => ({ ...current, retry: current.retry + 1 })),
  };
}

interface HistoryDetailState {
  history: EdgeHistory | null;
  loading: boolean;
  pathsLoading: boolean;
  pathEventsLoaded: number;
  pathEventsComplete: boolean;
  error: string | null;
  retry: number;
}

export function useHistoryDetail(
  edgeID: string | undefined,
  window: HistoryURLState["window"],
) {
  const [state, setState] = useState<HistoryDetailState>({
    history: null,
    loading: false,
    pathsLoading: false,
    pathEventsLoaded: 0,
    pathEventsComplete: false,
    error: null,
    retry: 0,
  });
  useEffect(() => {
    if (!edgeID) {
      setState((current) => ({
        ...current,
        history: null,
        loading: false,
        pathsLoading: false,
        pathEventsLoaded: 0,
        pathEventsComplete: false,
        error: null,
      }));
      return;
    }
    const controller = new AbortController();
    setState((current) => ({
      ...current,
      history: null,
      loading: true,
      pathsLoading: false,
      pathEventsLoaded: 0,
      pathEventsComplete: false,
      error: null,
    }));
    void getEdgeHistory(edgeID, controller.signal, window)
      .then(async (history) => {
        if (controller.signal.aborted) return;
        setState((current) => ({
          ...current,
          history,
          loading: false,
          pathsLoading: true,
          pathEventsLoaded: 0,
        }));
        let cursor = "";
        let anchor = history.pathAnchor;
        const events: EdgeHistory["pathEvents"] = [];
        try {
          do {
            const page = await getEdgePathHistory(
              edgeID,
              window,
              cursor,
              controller.signal,
            );
            if (controller.signal.aborted) return;
            if (!cursor && page.anchor) anchor = page.anchor;
            events.push(...page.events);
            cursor = page.nextCursor ?? "";
            const nextHistory = {
              ...history,
              pathAnchor: anchor,
              pathEvents: [...events],
              pathEventsTruncated: false,
            };
            setState((current) => ({
              ...current,
              history: nextHistory,
              pathsLoading: Boolean(cursor),
              pathEventsLoaded: events.length,
              pathEventsComplete: !cursor,
            }));
          } while (cursor);
        } catch (error: unknown) {
          if (controller.signal.aborted) return;
          // Older servers do not expose the paginated endpoint. Keep their
          // embedded compatibility history instead of making History unusable.
          if (error instanceof Error && error.message.startsWith("404 ")) {
            setState((current) => ({
              ...current,
              history,
              pathsLoading: false,
              pathEventsLoaded: history.pathEvents.length,
              pathEventsComplete: !history.pathEventsTruncated,
            }));
            return;
          }
          throw error;
        }
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setState((current) => ({
          ...current,
          loading: false,
          pathsLoading: false,
          error: error instanceof Error ? error.message : "History unavailable",
        }));
      });
    return () => controller.abort();
  }, [edgeID, state.retry, window]);
  return {
    ...state,
    retryRequest: () =>
      setState((current) => ({ ...current, retry: current.retry + 1 })),
  };
}
