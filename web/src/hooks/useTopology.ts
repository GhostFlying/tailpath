import { useCallback, useEffect, useRef, useState } from "react";
import { getTopology } from "../api/client";
import type { Topology } from "../api/types";

export type ConnectionState = "connecting" | "live" | "reconnecting" | "error";

export function useTopology() {
  const [topology, setTopology] = useState<Topology | null>(null);
  const [connection, setConnection] = useState<ConnectionState>("connecting");
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef<AbortController | null>(null);

  const refresh = useCallback(async () => {
    requestRef.current?.abort();
    const controller = new AbortController();
    requestRef.current = controller;
    try {
      const next = await getTopology(controller.signal);
      setTopology(next);
      setError(null);
    } catch (caught) {
      if (controller.signal.aborted) return;
      setError(
        caught instanceof Error ? caught.message : "Topology request failed",
      );
      setConnection("error");
    }
  }, []);

  useEffect(() => {
    void refresh();
    const events = new EventSource("/api/v1/events");
    events.addEventListener("ready", () => {
      setConnection("live");
      void refresh();
    });
    events.addEventListener("topology", () => {
      setConnection("live");
      void refresh();
    });
    events.onerror = () => setConnection("reconnecting");
    return () => {
      events.close();
      requestRef.current?.abort();
    };
  }, [refresh]);

  useEffect(() => {
    if (!topology) return;
    const periodicRefresh = 30_000;
    const deadline = topology.nextChangeAt
      ? new Date(topology.nextChangeAt).getTime() - Date.now() + 50
      : periodicRefresh;
    const delay = Math.min(Math.max(deadline, 250), periodicRefresh);
    const timer = window.setTimeout(() => void refresh(), delay);
    return () => window.clearTimeout(timer);
  }, [topology, refresh]);

  return { topology, connection, error, refresh };
}
