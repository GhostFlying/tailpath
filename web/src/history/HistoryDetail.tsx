import {
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  CircleAlert,
  Waypoints,
} from "lucide-react";
import { memo, useEffect, useMemo, useState } from "react";
import type { EdgeHistory, HistoryWindow, PathObservation } from "../api/types";
import { formatAgo, formatBytes, pathLabel } from "../lib/format";
import { IdentityBadge } from "../lib/identity";
import { DirectionalTrafficChart } from "./DirectionalTrafficChart";
import { PathTimeline } from "./PathTimeline";

const windows: HistoryWindow[] = ["15m", "1h", "6h", "24h", "7d"];

interface Props {
  history: EdgeHistory | null;
  loading: boolean;
  pathsLoading: boolean;
  pathEventsLoaded: number;
  pathEventsComplete: boolean;
  error: string | null;
  window: HistoryWindow;
  onBack: () => void;
  onRetry: () => void;
  onWindowChange: (window: HistoryWindow) => void;
  mobile: boolean;
}

export const HistoryDetail = memo(function HistoryDetail({
  history,
  loading,
  pathsLoading,
  pathEventsLoaded,
  pathEventsComplete,
  error,
  window,
  onBack,
  onRetry,
  onWindowChange,
  mobile,
}: Props) {
  const [selectedPathAt, setSelectedPathAt] = useState<string>();
  useEffect(() => setSelectedPathAt(undefined), [history?.edgeId, window]);
  const totals = useMemo(() => {
    let aToB = 0;
    let bToA = 0;
    for (const point of history?.traffic ?? []) {
      aToB += point.aToBBytes;
      bToA += point.bToABytes;
    }
    return { aToB, bToA };
  }, [history?.traffic]);
  const lastEvent = history
    ? (history.pathEvents.at(-1) ?? history.pathAnchor)
    : undefined;
  const lastPath = lastEvent?.path;
  const lastPathSummary = summarizeLastPath(lastEvent);
  const lastTraffic = history?.lastTrafficAt;

  return (
    <article className="history-detail-pane" aria-label="History edge detail">
      {loading ? (
        <div className="history-detail-state" aria-label="Loading edge history">
          <span className="loading-ring" />
          <strong>Loading connection</strong>
        </div>
      ) : null}
      {error ? (
        <div className="history-detail-state error-state">
          <CircleAlert size={24} />
          <strong>Connection history unavailable</strong>
          <span>{error}</span>
          <button type="button" onClick={onRetry}>
            Retry
          </button>
        </div>
      ) : null}
      {!loading && !error && !history ? (
        <div className="history-detail-state">
          <Waypoints size={25} />
          <strong>Select a connection</strong>
        </div>
      ) : null}
      {history && !loading && !error ? (
        <div className="history-detail-content">
          <header className="history-detail-header">
            <button
              type="button"
              className="history-back-button"
              onClick={onBack}
              aria-label="Back to connections"
            >
              <ArrowLeft size={22} />
            </button>
            <h1>
              {history.source.label} <span>↔</span> {history.target.label}
            </h1>
            <span
              className="history-detail-status"
              aria-label="Server reachable"
            />
          </header>
          {history.source.identityStatus || history.target.identityStatus ? (
            <div
              className="history-identity-pair"
              aria-label="Endpoint identities"
            >
              <span>{history.source.label}</span>
              <IdentityBadge status={history.source.identityStatus} compact />
              <span>{history.target.label}</span>
              <IdentityBadge status={history.target.identityStatus} compact />
            </div>
          ) : null}
          <div className="detail-window-control" aria-label="History window">
            {windows.map((item) => (
              <button
                key={item}
                type="button"
                aria-pressed={item === window}
                className={item === window ? "selected" : ""}
                onClick={() => onWindowChange(item)}
              >
                {item}
              </button>
            ))}
          </div>
          {history.traffic.length === 0 ? (
            <div className="history-detail-empty">
              <Waypoints size={27} />
              <strong>No traffic in this window</strong>
              <span>Choose a longer window to see earlier activity.</span>
            </div>
          ) : (
            <>
              <div className="history-detail-summary">
                <span>Last path</span>
                <strong
                  className={`path-text ${lastPathSummary.asymmetric ? "asymmetric" : (lastPath?.kind ?? "unknown")}`}
                >
                  {lastPathSummary.label}
                </strong>
                <i />
                <span>Last traffic</span>
                <strong>
                  {lastTraffic ? (
                    <time dateTime={lastTraffic}>{formatAgo(lastTraffic)}</time>
                  ) : (
                    "No traffic"
                  )}
                </strong>
                <i />
                <span className="history-total">
                  <ArrowUp size={14} /> {formatBytes(totals.aToB)}
                </span>
                <i />
                <span className="history-total">
                  <ArrowDown size={14} /> {formatBytes(totals.bToA)}
                </span>
              </div>
              <DirectionalTrafficChart
                history={history}
                selectedAt={selectedPathAt}
              />
              <PathTimeline
                history={history}
                mobile={mobile}
                loading={pathsLoading}
                loaded={pathEventsLoaded}
                complete={pathEventsComplete}
                onSelectTime={setSelectedPathAt}
              />
              {history.trafficTruncated || history.pathEventsTruncated ? (
                <p className="history-truncation-note">
                  Latest retained points shown
                </p>
              ) : null}
            </>
          )}
        </div>
      ) : null}
    </article>
  );
});

function summarizeLastPath(event: EdgeHistory["pathAnchor"] | undefined) {
  const directions = event?.directions ?? [];
  if (directions.length === 2) {
    const keys = directions.map(
      (direction) =>
        `${historyPathKey(direction.primaryPath)}|${direction.fallbackPath ? historyPathKey(direction.fallbackPath) : "none"}`,
    );
    if (keys[0] !== keys[1]) {
      return { label: "Asymmetric paths", asymmetric: true };
    }
    return {
      label: pathLabel(directions[0].primaryPath),
      asymmetric: false,
    };
  }
  return {
    label: event ? pathLabel(event.path) : "Unknown",
    asymmetric: false,
  };
}

function historyPathKey(path: PathObservation) {
  if (path.kind === "direct") return "direct";
  if (path.kind === "derp") return `derp:${path.derpRegion ?? "unknown"}`;
  if (path.kind === "peer_relay") {
    return `peer-relay:${path.peerRelayStableNodeId ?? path.peerRelayEndpoint ?? path.peerRelayVni ?? "unknown"}`;
  }
  return "unknown";
}
