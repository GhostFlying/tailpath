import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { getCapabilities } from "../api/client";
import type { ServerCapabilities } from "../api/types";

interface CapabilityState {
  capabilities: ServerCapabilities | null;
  loading: boolean;
  error: string | null;
  deviceDirectoryEnabled: boolean;
  retry: () => void;
}

const CapabilityContext = createContext<CapabilityState | null>(null);

export function CapabilityProvider({ children }: { children: ReactNode }) {
  const [capabilities, setCapabilities] = useState<ServerCapabilities | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [request, setRequest] = useState(0);

  const retry = useCallback(() => setRequest((value) => value + 1), []);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    void getCapabilities(controller.signal)
      .then((next) => {
        setCapabilities(next);
        setError(null);
      })
      .catch((caught) => {
        if (controller.signal.aborted) return;
        setError(
          caught instanceof Error
            ? caught.message
            : "Capabilities request failed",
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [request]);

  const value = useMemo<CapabilityState>(
    () => ({
      capabilities,
      loading,
      error,
      deviceDirectoryEnabled:
        capabilities?.features.includes("device-directory") ?? false,
      retry,
    }),
    [capabilities, error, loading, retry],
  );
  return (
    <CapabilityContext.Provider value={value}>
      {children}
    </CapabilityContext.Provider>
  );
}

export function useCapabilities(): CapabilityState {
  const value = useContext(CapabilityContext);
  if (!value) {
    throw new Error("useCapabilities must be used inside CapabilityProvider");
  }
  return value;
}
