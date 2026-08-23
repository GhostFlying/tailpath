import { lazy, Suspense } from "react";
import { BrowserRouter, Route, Routes } from "react-router-dom";

const LiveWorkspace = lazy(() => import("./LiveWorkspace"));
const HistoryWorkspace = lazy(() => import("./history/HistoryWorkspace"));

export default function App() {
  return (
    <BrowserRouter>
      <Suspense fallback={<RouteLoading />}>
        <Routes>
          <Route path="/" element={<LiveWorkspace />} />
          <Route path="/history" element={<HistoryWorkspace />} />
          <Route path="/history/edges/:edgeId" element={<HistoryWorkspace />} />
        </Routes>
      </Suspense>
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
