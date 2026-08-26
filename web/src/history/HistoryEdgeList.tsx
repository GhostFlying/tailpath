import {
  Activity,
  ArrowDown,
  ArrowUp,
  ChevronRight,
  CircleAlert,
  Waypoints,
} from "lucide-react";
import { memo } from "react";
import type { HistoryEdgePage, HistoryEdgeSummary } from "../api/types";
import { formatAgo, formatBytes } from "../lib/format";
import { IdentityBadge } from "../lib/identity";
import { platformPresentation } from "../lib/platform";

interface Props {
  page: HistoryEdgePage | null;
  selectedEdgeID?: string;
  loading: boolean;
  error: string | null;
  onSelect: (edgeID: string) => void;
  onRetry: () => void;
  onNextPage: (cursor: string) => void;
}

export function HistoryEdgeList({
  page,
  selectedEdgeID,
  loading,
  error,
  onSelect,
  onRetry,
  onNextPage,
}: Props) {
  return (
    <section className="history-list-pane" aria-label="History connections">
      <header className="history-list-heading">
        <strong>Connections</strong>
        <span>Last traffic</span>
        <span>Total</span>
      </header>
      <div className="history-edge-list">
        {loading ? <HistoryListLoading /> : null}
        {error ? (
          <HistoryListState
            icon={<CircleAlert size={22} />}
            title="History unavailable"
            detail={error}
            action="Retry"
            onAction={onRetry}
          />
        ) : null}
        {!loading && !error && page?.edges.length === 0 ? (
          <HistoryListState
            icon={<Waypoints size={23} />}
            title="No matching traffic"
            detail="No connection had traffic in this window."
          />
        ) : null}
        {!loading && !error
          ? page?.edges.map((edge) => (
              <HistoryEdgeRow
                key={edge.edgeId}
                edge={edge}
                selected={edge.edgeId === selectedEdgeID}
                onSelect={onSelect}
              />
            ))
          : null}
      </div>
      {page?.nextCursor && !loading && !error ? (
        <button
          className="history-next-page"
          type="button"
          onClick={() => onNextPage(page.nextCursor!)}
        >
          Next page
          <ChevronRight size={16} />
        </button>
      ) : null}
    </section>
  );
}

interface RowProps {
  edge: HistoryEdgeSummary;
  selected: boolean;
  onSelect: (edgeID: string) => void;
}

const HistoryEdgeRow = memo(function HistoryEdgeRow({
  edge,
  selected,
  onSelect,
}: RowProps) {
  const platform = platformPresentation(edge.source.os);
  const path = summaryPath(edge);
  return (
    <button
      type="button"
      className={`history-edge-row ${selected ? "selected" : ""}`}
      onClick={() => onSelect(edge.edgeId)}
      aria-current={selected ? "true" : undefined}
    >
      <span className="history-row-platform" aria-label={platform.label}>
        <platform.Icon size={25} strokeWidth={1.7} />
      </span>
      <span className="history-row-identity">
        <strong>
          <span className="history-row-pair">
            {edge.source.label} ↔ {edge.target.label}
          </span>
          {selected ? (
            <Activity size={14} aria-label="Selected connection" />
          ) : null}
        </strong>
        <span className="history-row-subline">
          <small className={`path-text ${path.kind}`}>{path.label}</small>
          <IdentityBadge status={edge.source.identityStatus} compact />
          <IdentityBadge status={edge.target.identityStatus} compact />
        </span>
      </span>
      <span className="history-row-metadata">
        <time dateTime={edge.lastTrafficAt}>
          {formatAgo(edge.lastTrafficAt)}
        </time>
        <span className="history-row-totals">
          <span>
            <ArrowUp size={13} /> {formatBytes(edge.aToBBytes)}
          </span>
          <span>
            <ArrowDown size={13} /> {formatBytes(edge.bToABytes)}
          </span>
        </span>
      </span>
      <ChevronRight className="history-row-chevron" size={19} />
    </button>
  );
});

function summaryPath(edge: HistoryEdgeSummary) {
  if (edge.paths.length === 0) return { kind: "unknown", label: "Unknown" };
  if (edge.paths.length > 1) {
    return { kind: "multiple", label: `${edge.paths.length} paths seen` };
  }
  switch (edge.paths[0]) {
    case "direct":
      return { kind: "direct", label: "Direct" };
    case "derp":
      return { kind: "derp", label: "DERP" };
    case "peer_relay":
      return { kind: "peer_relay", label: "Peer Relay" };
    default:
      return { kind: "unknown", label: "Unknown" };
  }
}

function HistoryListLoading() {
  return (
    <div className="history-list-loading" aria-label="Loading history">
      {Array.from({ length: 5 }, (_, index) => (
        <span key={index} />
      ))}
    </div>
  );
}

function HistoryListState({
  icon,
  title,
  detail,
  action,
  onAction,
}: {
  icon: React.ReactNode;
  title: string;
  detail: string;
  action?: string;
  onAction?: () => void;
}) {
  return (
    <div className="history-list-state">
      {icon}
      <strong>{title}</strong>
      <span>{detail}</span>
      {action ? (
        <button type="button" onClick={onAction}>
          {action}
        </button>
      ) : null}
    </div>
  );
}
