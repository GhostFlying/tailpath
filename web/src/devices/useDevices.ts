import { useCallback, useEffect, useRef, useState } from "react";
import { getDevices, getTopology } from "../api/client";
import type { DeviceDirectory } from "../api/types";
import { visibleTopologyNodeIDs } from "../lib/graph";
import { createSingleFlight } from "../lib/singleFlight";
import { readShowRecentPreference } from "../lib/uiPreferences";

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
  const [liveVisibleNodeIDs, setLiveVisibleNodeIDs] = useState<Set<string>>(
    () => new Set(),
  );
  const refreshRef = useRef<() => Promise<void>>(() => Promise.resolve());
  const refresh = useCallback(() => refreshRef.current(), []);

  useEffect(() => {
    if (!enabled) {
      setDirectory(null);
      setError(null);
      setConnection("connecting");
      setLiveVisibleNodeIDs(new Set());
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
    const visibilityRunner = createSingleFlight(async () => {
      try {
        const topology = await getTopology();
        setLiveVisibleNodeIDs(
          visibleTopologyNodeIDs(
            topology,
            "all",
            readShowRecentPreference(window.localStorage),
          ),
        );
      } catch {
        // Cross-workspace navigation is best effort and never blocks the directory.
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
        void visibilityRunner.request();
        return;
      }
      invalidationTimer = window.setTimeout(() => {
        invalidationTimer = null;
        void runner.request();
        void visibilityRunner.request();
      }, delay);
    };
    refreshRef.current = runner.request;
    void runner.request();
    void visibilityRunner.request();
    const events = new EventSource("/api/v1/events");
    events.addEventListener("ready", () => {
      setConnection("reachable");
      void runner.request();
      void visibilityRunner.request();
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
      visibilityRunner.stop();
      if (invalidationTimer !== null) window.clearTimeout(invalidationTimer);
      controller?.abort();
    };
  }, [enabled]);

  return { directory, connection, error, refresh, liveVisibleNodeIDs };
}
