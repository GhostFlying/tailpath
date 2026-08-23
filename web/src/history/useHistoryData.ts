import { useEffect, useState } from "react";
import {
  getEdgeHistory,
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
    error: null,
    retry: 0,
  });
  useEffect(() => {
    if (!edgeID) {
      setState((current) => ({
        ...current,
        history: null,
        loading: false,
        error: null,
      }));
      return;
    }
    const controller = new AbortController();
    setState((current) => ({
      ...current,
      history: null,
      loading: true,
      error: null,
    }));
    void getEdgeHistory(edgeID, controller.signal, window)
      .then((history) => {
        setState((current) => ({
          ...current,
          history,
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
  }, [edgeID, state.retry, window]);
  return {
    ...state,
    retryRequest: () =>
      setState((current) => ({ ...current, retry: current.retry + 1 })),
  };
}
