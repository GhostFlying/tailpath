import { CircleAlert, Waypoints } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import type { PathKind } from "./api/types";
import { GraphLegend } from "./components/GraphLegend";
import { Inspector } from "./components/Inspector";
import { TopologyFilters } from "./components/TopologyFilters";
import { TopologyGraph } from "./components/TopologyGraph";
import {
  WorkspaceTopbar,
  type WorkspaceConnection,
} from "./components/WorkspaceTopbar";
import { useTopology, type ConnectionState } from "./hooks/useTopology";
import {
  edgeIsVisible,
  emptyTrafficReason,
  visibleTopologyNodeIDs,
  type EmptyTrafficReason,
  type PathFilter,
} from "./lib/graph";
import {
  readShowRecentPreference,
  writeShowRecentPreference,
} from "./lib/uiPreferences";

export default function LiveWorkspace() {
  const { topology, connection, error, refresh } = useTopology();
  const [searchParams, setSearchParams] = useSearchParams();
  const [pathFilter, setPathFilter] = useState<PathFilter>("all");
  const [showRecent, setShowRecent] = useState(() =>
    readShowRecentPreference(
      typeof window === "undefined" ? undefined : window.localStorage,
    ),
  );
  const [query, setQuery] = useState("");
  const [selectedEdgeId, setSelectedEdgeId] = useState<string | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [focusNodeId, setFocusNodeId] = useState<string | null>(null);
  const selectedEdge =
    topology?.edges.find((edge) => edge.id === selectedEdgeId) ?? null;
  const selectedNode =
    topology?.nodes.find((node) => node.id === selectedNodeId) ?? null;
  const countedEdges = useMemo(
    () =>
      (topology?.edges ?? []).filter(
        (edge) =>
          (showRecent || edge.state === "active") && !edge.systemTelemetry,
      ),
    [topology, showRecent],
  );
  const counts = useMemo(() => pathCounts(countedEdges), [countedEdges]);
  const visibleNodeIDs = useMemo(
    () =>
      topology
        ? visibleTopologyNodeIDs(topology, pathFilter, showRecent)
        : new Set<string>(),
    [pathFilter, showRecent, topology],
  );
  const emptyReason = topology
    ? emptyTrafficReason(topology.edges, pathFilter, showRecent)
    : null;
  const skewedRuntimes =
    topology?.observers.filter((observer) => observer.clockSkewed).length ?? 0;
  const liveRuntimes =
    topology?.observers.filter((observer) => observer.online).length ?? 0;
  const totalRuntimes = topology?.observers.length ?? 0;
  const staleRuntimes = totalRuntimes - liveRuntimes;

  useEffect(() => {
    if (selectedEdge && !edgeIsVisible(selectedEdge, pathFilter, showRecent)) {
      setSelectedEdgeId(null);
    }
  }, [pathFilter, selectedEdge, showRecent]);

  useEffect(() => {
    if (selectedNodeId && !visibleNodeIDs.has(selectedNodeId)) {
      setSelectedNodeId(null);
    }
  }, [selectedNodeId, visibleNodeIDs]);

  useEffect(() => {
    if (!topology) return;
    const requestedNodeID = searchParams.get("nodeId");
    if (!requestedNodeID) return;
    if (visibleNodeIDs.has(requestedNodeID)) {
      setSelectedEdgeId(null);
      setSelectedNodeId(requestedNodeID);
      setFocusNodeId(requestedNodeID);
    }
    const next = new URLSearchParams(searchParams);
    next.delete("nodeId");
    setSearchParams(next, { replace: true });
  }, [searchParams, setSearchParams, topology, visibleNodeIDs]);

  function updateShowRecent(next: boolean) {
    setShowRecent(next);
    writeShowRecentPreference(
      typeof window === "undefined" ? undefined : window.localStorage,
      next,
    );
  }

  return (
    <main className="app-shell">
      <WorkspaceTopbar
        connection={liveConnection(connection)}
        metrics={
          <>
            <span>
              <strong>{topology?.nodes.length ?? 0}</strong> nodes
            </span>
            <span>
              <strong>
                {countedEdges.filter((edge) => edge.state === "active").length}
              </strong>{" "}
              active edges
            </span>
            <span>
              <strong>{liveRuntimes}</strong> live runtimes
            </span>
          </>
        }
      />

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
          staleRuntimes={staleRuntimes}
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
              selectedNodeId={selectedNodeId}
              focusNodeId={focusNodeId}
              onSelectEdge={setSelectedEdgeId}
              onSelectNode={(nodeID) => {
                setFocusNodeId(null);
                setSelectedNodeId(nodeID);
              }}
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

function liveConnection(connection: ConnectionState): WorkspaceConnection {
  switch (connection) {
    case "live":
      return {
        state: "live",
        label: "Live",
        ariaLabel: "Live updates connected",
      };
    case "reconnecting":
      return {
        state: "reconnecting",
        label: "Reconnecting",
        ariaLabel: "Live updates reconnecting",
      };
    case "error":
      return {
        state: "error",
        label: "Unavailable",
        ariaLabel: "Live topology unavailable",
      };
    default:
      return {
        state: "connecting",
        label: "Connecting",
        ariaLabel: "Live updates connecting",
      };
  }
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
