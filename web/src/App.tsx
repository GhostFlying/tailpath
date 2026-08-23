import { CircleAlert, Waypoints } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { PathKind } from "./api/types";
import { GraphLegend } from "./components/GraphLegend";
import { Inspector } from "./components/Inspector";
import { TopologyFilters } from "./components/TopologyFilters";
import { TopologyGraph } from "./components/TopologyGraph";
import { useTopology } from "./hooks/useTopology";
import {
  edgeIsVisible,
  emptyTrafficReason,
  type EmptyTrafficReason,
  type PathFilter,
} from "./lib/graph";
import {
  readShowRecentPreference,
  writeShowRecentPreference,
} from "./lib/uiPreferences";

export default function App() {
  const { topology, connection, error, refresh } = useTopology();
  const [pathFilter, setPathFilter] = useState<PathFilter>("all");
  const [showRecent, setShowRecent] = useState(() =>
    readShowRecentPreference(
      typeof window === "undefined" ? undefined : window.localStorage,
    ),
  );
  const [query, setQuery] = useState("");
  const [selectedEdgeId, setSelectedEdgeId] = useState<string | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const selectedEdge =
    topology?.edges.find((edge) => edge.id === selectedEdgeId) ?? null;
  const selectedNode =
    topology?.nodes.find((node) => node.id === selectedNodeId) ?? null;
  const countedEdges = useMemo(
    () =>
      (topology?.edges ?? []).filter(
        (edge) => showRecent || edge.state === "active",
      ),
    [topology, showRecent],
  );
  const counts = useMemo(() => pathCounts(countedEdges), [countedEdges]);
  const emptyReason = topology
    ? emptyTrafficReason(topology.edges, pathFilter, showRecent)
    : null;
  const skewedRuntimes =
    topology?.observers.filter((observer) => observer.clockSkewed).length ?? 0;
  const liveRuntimes =
    topology?.observers.filter((observer) => observer.online).length ?? 0;
  const totalRuntimes = topology?.observers.length ?? 0;

  useEffect(() => {
    if (selectedEdge && !edgeIsVisible(selectedEdge, pathFilter, showRecent)) {
      setSelectedEdgeId(null);
    }
  }, [pathFilter, selectedEdge, showRecent]);

  function updateShowRecent(next: boolean) {
    setShowRecent(next);
    writeShowRecentPreference(
      typeof window === "undefined" ? undefined : window.localStorage,
      next,
    );
  }

  return (
    <main className="app-shell">
      <header className="topbar">
        <div className="brand">
          <Waypoints size={22} />
          <strong>Tailpath</strong>
        </div>
        <div className="headline-metrics">
          <span>
            <strong>{topology?.nodes.length ?? 0}</strong> nodes
          </span>
          <span>
            <strong>
              {topology?.edges.filter((edge) => edge.state === "active")
                .length ?? 0}
            </strong>{" "}
            active edges
          </span>
          <span>
            <strong>
              {topology?.observers.filter((observer) => observer.online)
                .length ?? 0}
            </strong>{" "}
            live runtimes
          </span>
        </div>
        <div className={`live-state ${connection}`}>
          <span />
          {connection}
        </div>
      </header>

      <div className="workspace">
        <TopologyFilters
          pathFilter={pathFilter}
          onPathFilterChange={setPathFilter}
          query={query}
          onQueryChange={setQuery}
          showRecent={showRecent}
          onShowRecentChange={updateShowRecent}
          counts={counts}
          edgeCount={countedEdges.length}
          liveRuntimes={liveRuntimes}
          totalRuntimes={totalRuntimes}
          skewedRuntimes={skewedRuntimes}
        />

        <section className="graph-stage">
          {topology ? (
            <TopologyGraph
              topology={topology}
              pathFilter={pathFilter}
              showRecent={showRecent}
              query={query}
              selectedEdgeId={selectedEdgeId}
              onSelectEdge={setSelectedEdgeId}
              onSelectNode={setSelectedNodeId}
            />
          ) : null}
          {topology ? <GraphLegend /> : null}
          {!topology && !error ? (
            <div className="center-state">
              <span className="loading-ring" />
              <strong>Loading topology</strong>
            </div>
          ) : null}
          {error ? (
            <div className="center-state error-state">
              <CircleAlert size={24} />
              <strong>Topology unavailable</strong>
              <span>{error}</span>
              <button onClick={() => void refresh()}>Retry</button>
            </div>
          ) : null}
          {topology && emptyReason ? (
            <div className="center-state">
              <Waypoints size={26} />
              <strong>{emptyTrafficCopy[emptyReason].title}</strong>
              <span>{emptyTrafficCopy[emptyReason].detail}</span>
            </div>
          ) : null}
        </section>

        {topology ? (
          <Inspector
            topology={topology}
            edge={selectedEdge}
            node={selectedNode}
            onClose={() => {
              setSelectedEdgeId(null);
              setSelectedNodeId(null);
            }}
          />
        ) : null}
      </div>
    </main>
  );
}

const emptyTrafficCopy: Record<
  EmptyTrafficReason,
  { title: string; detail: string }
> = {
  "no-active": {
    title: "No active traffic",
    detail: "No matching relationship is active right now.",
  },
  "no-recent": {
    title: "No recent traffic",
    detail: "No matching relationship was active in the recent window.",
  },
  "no-match": {
    title: "No matching traffic",
    detail: "No recent relationship matches the selected path.",
  },
};

function pathCounts(edges: { path: { kind: PathKind } }[]) {
  return edges.reduce<Record<PathKind, number>>(
    (counts, edge) => {
      counts[edge.path.kind] += 1;
      return counts;
    },
    { direct: 0, derp: 0, peer_relay: 0, unknown: 0 },
  );
}
