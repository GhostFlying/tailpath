import { useCallback, useEffect, useRef, useState } from "react";
import { getTopology } from "../api/client";
import type { Topology } from "../api/types";
import { createSingleFlight } from "../lib/singleFlight";

export type ConnectionState = "connecting" | "live" | "reconnecting" | "error";

export function useTopology() {
  const [topology, setTopology] = useState<Topology | null>(null);
  const [connection, setConnection] = useState<ConnectionState>("connecting");
  const [error, setError] = useState<string | null>(null);
  const refreshRef = useRef<() => Promise<void>>(() => Promise.resolve());

  const refresh = useCallback(() => refreshRef.current(), []);

  useEffect(() => {
    let controller: AbortController | null = null;
    const runner = createSingleFlight(async () => {
      const requestController = new AbortController();
      controller = requestController;
      try {
        const next = await getTopology(requestController.signal);
        setTopology(next);
        setError(null);
      } catch (caught) {
        if (requestController.signal.aborted) return;
        setError(
          caught instanceof Error ? caught.message : "Topology request failed",
        );
        setConnection("error");
      } finally {
        if (controller === requestController) controller = null;
      }
    });
    refreshRef.current = runner.request;
    void runner.request();
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
      refreshRef.current = () => Promise.resolve();
      runner.stop();
      controller?.abort();
    };
  }, []);

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
