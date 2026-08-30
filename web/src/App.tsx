import { lazy, Suspense } from "react";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import { CapabilityProvider } from "./hooks/useCapabilities";

const LiveWorkspace = lazy(() => import("./LiveWorkspace"));
const HistoryWorkspace = lazy(() => import("./history/HistoryWorkspace"));
const DevicesWorkspace = lazy(() => import("./devices/DevicesWorkspace"));

export default function App() {
  return (
    <BrowserRouter>
      <CapabilityProvider>
        <Suspense fallback={<RouteLoading />}>
          <Routes>
            <Route path="/" element={<LiveWorkspace />} />
            <Route path="/history" element={<HistoryWorkspace />} />
            <Route
              path="/history/edges/:edgeId"
              element={<HistoryWorkspace />}
            />
            <Route path="/devices" element={<DevicesWorkspace />} />
            <Route path="/devices/:nodeId" element={<DevicesWorkspace />} />
          </Routes>
        </Suspense>
      </CapabilityProvider>
    </BrowserRouter>
  );
}

function RouteLoading() {
  return (
    <main className="route-loading" aria-label="Loading workspace">
      <span className="loading-ring" />
    </main>
  );
}
