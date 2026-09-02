import {
  Activity,
  ChevronRight,
  CircleHelp,
  Globe2,
  RadioTower,
  TriangleAlert,
  Users,
  X,
} from "lucide-react";
import {
  memo,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import type { EdgeHistory, HistoryNodeReference, PathKind } from "../api/types";
import { pathLabel } from "../lib/format";
import { identityPresentation } from "../lib/identity";
import {
  buildPathTimeline,
  pathColor,
  pathEvidenceKey,
  type PathTimelineItem,
} from "./historyMath";

interface Props {
  history: EdgeHistory;
  mobile: boolean;
}

export const PathTimeline = memo(function PathTimeline({
  history,
  mobile,
}: Props) {
  const items = useMemo(() => buildPathTimeline(history), [history]);
  const [selectedID, setSelectedID] = useState("");
  const [sheetOpen, setSheetOpen] = useState(false);
  const closeSheet = useCallback(() => setSheetOpen(false), []);
  const selected =
    items.find((item) => item.id === selectedID) ??
    items.find((item) => !item.anchored && item.observations.length > 0) ??
    items[0];
  const nodes = useMemo(() => buildHistoryNodeMaps(history), [history]);

  useEffect(() => {
    if (!mobile) setSheetOpen(false);
  }, [mobile]);

  function select(item: PathTimelineItem) {
    setSelectedID(item.id);
    if (mobile) setSheetOpen(true);
  }

  return (
    <section className="history-section path-timeline-section">
      <div className="history-section-heading">
        <h2>Path timeline</h2>
        <span>Newest first</span>
      </div>
      {items.length === 0 ? (
        <div className="history-chart-empty">
          No path evidence in this window
        </div>
      ) : (
        <div className="path-timeline" role="list" aria-label="Path timeline">
          {items.map((item) => {
            const Icon = pathIcon(item.path.kind);
            const active = item.id === selected?.id;
            return (
              <button
                key={item.id}
                type="button"
                role="listitem"
                className={active ? "selected" : ""}
                style={
                  {
                    "--timeline-color": pathColor(item.path.kind),
                    "--timeline-grow": Math.max(1, item.durationMs),
                  } as React.CSSProperties
                }
                aria-pressed={active}
                onClick={() => select(item)}
              >
                <span className="timeline-time">
                  <strong>{formatTimelineTime(item.from)}</strong>
                  <small>
                    {formatRelativeBoundary(item.from, history.from)}
                  </small>
                </span>
                <span className="timeline-symbol">
                  <Icon size={19} />
                </span>
                <span className="timeline-copy">
                  <strong>
                    {displayPathLabel(item.path, nodes.byStableID)}
                  </strong>
                  <small>{formatDuration(item.durationMs)}</small>
                </span>
                <span className="timeline-observers">
                  <Users size={15} /> {item.observations.length} observers
                </span>
                <ChevronRight size={18} />
              </button>
            );
          })}
        </div>
      )}
      {selected && !mobile ? (
        <ProvenanceContent
          history={history}
          selected={selected}
          nodes={nodes}
        />
      ) : null}
      {selected && mobile && sheetOpen ? (
        <MobileProvenanceSheet
          history={history}
          selected={selected}
          nodes={nodes}
          onClose={closeSheet}
        />
      ) : null}
    </section>
  );
});

interface HistoryNodeMaps {
  byID: Map<string, HistoryNodeReference>;
  byStableID: Map<string, HistoryNodeReference>;
}

function buildHistoryNodeMaps(history: EdgeHistory): HistoryNodeMaps {
  const byID = new Map<string, HistoryNodeReference>();
  const byStableID = new Map<string, HistoryNodeReference>();
  for (const node of [
    history.source,
    history.target,
    ...history.relatedNodes,
  ]) {
    byID.set(node.id, node);
    if (node.stableNodeId) byStableID.set(node.stableNodeId, node);
  }
  return { byID, byStableID };
}

function ProvenanceContent({
  history,
  selected,
  nodes,
  onClose,
  closeButtonRef,
}: {
  history: EdgeHistory;
  selected: PathTimelineItem;
  nodes: HistoryNodeMaps;
  onClose?: () => void;
  closeButtonRef?: RefObject<HTMLButtonElement | null>;
}) {
  return (
    <div className="provenance-section">
      <header className="provenance-title">
        <div>
          <span>Effective path at</span>
          <strong>{formatTimelineDateTime(selected.from)}</strong>
        </div>
        <span
          className={`path-text ${selected.path.kind}`}
          aria-label="Effective path"
        >
          {displayPathLabel(selected.path, nodes.byStableID)}
        </span>
        {onClose ? (
          <button
            ref={closeButtonRef}
            type="button"
            onClick={onClose}
            aria-label="Close path evidence"
          >
            <X size={20} />
          </button>
        ) : null}
      </header>
      <h2>Observed by</h2>
      {selected.observations.length === 0 ? (
        <div className="provenance-empty">No observer provenance retained</div>
      ) : (
        <div className="provenance-table" role="table" aria-label="Observed by">
          <div className="provenance-head" role="row">
            <span>Node</span>
            <span>Evidence</span>
            <span>Observed at</span>
            <span>Received at</span>
          </div>
          {selected.observations.map((observation, index) => {
            const node = nodes.byID.get(observation.observerId);
            const supports =
              pathEvidenceKey(observation.path) ===
              pathEvidenceKey(selected.path);
            return (
              <div
                className={`provenance-row ${observation.relaySession ? "relay-session" : ""}`}
                role="row"
                key={`${observation.observerId}:${index}`}
              >
                <span>
                  {node?.label ?? observation.observerId}
                  {observation.clockSkewed ? (
                    <TriangleAlert
                      size={14}
                      aria-label="Collector clock warning"
                    />
                  ) : null}
                </span>
                <span
                  className={
                    supports ? "evidence-support" : "evidence-conflict"
                  }
                >
                  {supports ? "Supports selected path" : "Conflicts"}:{" "}
                  {displayPathLabel(observation.path, nodes.byStableID)}
                </span>
                <time dateTime={observation.collectedAt}>
                  {formatTimelineTime(observation.collectedAt, true)}
                </time>
                <time dateTime={observation.receivedAt}>
                  {formatTimelineTime(observation.receivedAt, true)}
                </time>
                {observation.relaySession ? (
                  <RelaySessionDetails
                    history={history}
                    observation={observation}
                    nodes={nodes}
                  />
                ) : null}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function RelaySessionDetails({
  history,
  observation,
  nodes,
}: {
  history: EdgeHistory;
  observation: PathTimelineItem["observations"][number];
  nodes: HistoryNodeMaps;
}) {
  const session = observation.relaySession;
  if (!session) return null;
  const relayLabel = observation.path.peerRelayStableNodeId
    ? (nodes.byStableID.get(observation.path.peerRelayStableNodeId)?.label ??
      observation.path.peerRelayStableNodeId)
    : "unknown";
  const bothResolved =
    session.sourceIdentityStatus === "resolved" &&
    session.targetIdentityStatus === "resolved";
  return (
    <div className="history-relay-provenance">
      <span>Relay: {relayLabel}</span>
      <span>VNI {session.vni}</span>
      <span>
        Session <code>{session.sessionId}</code>
      </span>
      {bothResolved ? (
        <span className="identity-resolution resolved">
          Both endpoints resolved
        </span>
      ) : (
        <>
          <EndpointIdentity
            label={history.source.label}
            status={session.sourceIdentityStatus}
          />
          <EndpointIdentity
            label={history.target.label}
            status={session.targetIdentityStatus}
          />
        </>
      )}
    </div>
  );
}

function EndpointIdentity({
  label,
  status,
}: {
  label: string;
  status: "resolved" | "partial" | "anonymous" | "conflict";
}) {
  const presentation = identityPresentation(status);
  if (!presentation) return null;
  return (
    <span className={`identity-resolution ${status}`}>
      {label}: {presentation.shortLabel}
    </span>
  );
}

function MobileProvenanceSheet({
  history,
  selected,
  nodes,
  onClose,
}: {
  history: EdgeHistory;
  selected: PathTimelineItem;
  nodes: HistoryNodeMaps;
  onClose: () => void;
}) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement | null;
    closeButtonRef.current?.focus();
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
        return;
      }
      if (event.key !== "Tab" || !dialogRef.current) return;
      const focusable = Array.from(
        dialogRef.current.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [href], [tabindex]:not([tabindex="-1"])',
        ),
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      previouslyFocused?.focus();
    };
  }, [onClose]);

  return createPortal(
    <div className="history-sheet-layer">
      <div
        className="history-sheet-backdrop"
        aria-hidden="true"
        onClick={onClose}
      />
      <div
        ref={dialogRef}
        className="history-provenance-sheet"
        role="dialog"
        aria-modal="true"
        aria-label="Path evidence"
      >
        <ProvenanceContent
          history={history}
          selected={selected}
          nodes={nodes}
          onClose={onClose}
          closeButtonRef={closeButtonRef}
        />
      </div>
    </div>,
    document.body,
  );
}

function displayPathLabel(
  path: PathTimelineItem["path"],
  byStableID: Map<string, HistoryNodeReference>,
) {
  if (path.kind !== "peer_relay" || !path.peerRelayStableNodeId) {
    return pathLabel(path);
  }
  const relay = byStableID.get(path.peerRelayStableNodeId);
  return `Peer Relay via ${relay?.label ?? path.peerRelayStableNodeId}`;
}

function pathIcon(kind: PathKind) {
  switch (kind) {
    case "direct":
      return Activity;
    case "derp":
      return Globe2;
    case "peer_relay":
      return RadioTower;
    default:
      return CircleHelp;
  }
}

function formatDuration(milliseconds: number) {
  const minutes = Math.max(0, Math.round(milliseconds / 60_000));
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  const remainder = minutes % 60;
  return remainder ? `${hours}h ${remainder}m` : `${hours}h`;
}

function formatTimelineTime(value: string, seconds = false) {
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: seconds ? "2-digit" : undefined,
    hour12: false,
  }).format(new Date(value));
}

function formatTimelineDateTime(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(new Date(value));
}

function formatRelativeBoundary(value: string, from: string) {
  if (value === from) return "window start";
  return "path change";
}
