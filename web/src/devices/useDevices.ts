import { useCallback, useEffect, useRef, useState } from "react";
import { getDevices } from "../api/client";
import type { DeviceDirectory } from "../api/types";
import { createSingleFlight } from "../lib/singleFlight";

export type DeviceConnectionState =
  | "connecting"
  | "reachable"
  | "reconnecting"
  | "error";

const invalidationFloorMs = 5_000;

export function useDevices(enabled: boolean) {
  const [directory, setDirectory] = useState<DeviceDirectory | null>(null);
  const [connection, setConnection] =
    useState<DeviceConnectionState>("connecting");
  const [error, setError] = useState<string | null>(null);
  const refreshRef = useRef<() => Promise<void>>(() => Promise.resolve());
  const refresh = useCallback(() => refreshRef.current(), []);

  useEffect(() => {
    if (!enabled) {
      setDirectory(null);
      setError(null);
      setConnection("connecting");
      return;
    }
    let controller: AbortController | null = null;
    let invalidationTimer: number | null = null;
    let lastRequestAt = 0;
    const runner = createSingleFlight(async () => {
      lastRequestAt = Date.now();
      const requestController = new AbortController();
      controller = requestController;
      try {
        const next = await getDevices(requestController.signal);
        setDirectory(next);
        setError(null);
        setConnection("reachable");
      } catch (caught) {
        if (requestController.signal.aborted) return;
        setError(
          caught instanceof Error
            ? caught.message
            : "Device directory request failed",
        );
        setConnection("error");
      } finally {
        if (controller === requestController) controller = null;
      }
    });
    const invalidate = () => {
      if (invalidationTimer !== null) return;
      const delay = Math.max(
        0,
        invalidationFloorMs - (Date.now() - lastRequestAt),
      );
      if (delay === 0) {
        void runner.request();
        return;
      }
      invalidationTimer = window.setTimeout(() => {
        invalidationTimer = null;
        void runner.request();
      }, delay);
    };
    refreshRef.current = runner.request;
    void runner.request();
    const events = new EventSource("/api/v1/events");
    events.addEventListener("ready", () => {
      setConnection("reachable");
      void runner.request();
    });
    events.addEventListener("topology", () => {
      setConnection("reachable");
      invalidate();
    });
    events.onerror = () => setConnection("reconnecting");
    return () => {
      events.close();
      refreshRef.current = () => Promise.resolve();
      runner.stop();
      if (invalidationTimer !== null) window.clearTimeout(invalidationTimer);
      controller?.abort();
    };
  }, [enabled]);

  return { directory, connection, error, refresh };
}
