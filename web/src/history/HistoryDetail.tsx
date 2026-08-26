import {
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  CircleAlert,
  Waypoints,
} from "lucide-react";
import { memo, useMemo } from "react";
import type { EdgeHistory, HistoryWindow } from "../api/types";
import { formatAgo, formatBytes, pathLabel } from "../lib/format";
import { IdentityBadge } from "../lib/identity";
import { DirectionalTrafficChart } from "./DirectionalTrafficChart";
import { PathTimeline } from "./PathTimeline";

const windows: HistoryWindow[] = ["15m", "1h", "6h", "24h", "7d"];

interface Props {
  history: EdgeHistory | null;
  loading: boolean;
  error: string | null;
  window: HistoryWindow;
  onBack: () => void;
  onRetry: () => void;
  onWindowChange: (window: HistoryWindow) => void;
}

export const HistoryDetail = memo(function HistoryDetail({
  history,
  loading,
  error,
  window,
  onBack,
  onRetry,
  onWindowChange,
}: Props) {
  const totals = useMemo(() => {
    let aToB = 0;
    let bToA = 0;
    for (const point of history?.traffic ?? []) {
      aToB += point.aToBBytes;
      bToA += point.bToABytes;
    }
    return { aToB, bToA };
  }, [history?.traffic]);
  const lastPath = history
    ? (history.pathEvents.at(-1)?.path ?? history.pathAnchor?.path)
    : undefined;
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
                <strong className={`path-text ${lastPath?.kind ?? "unknown"}`}>
                  {lastPath ? pathLabel(lastPath) : "Unknown"}
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
              <DirectionalTrafficChart history={history} />
              <PathTimeline history={history} />
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
