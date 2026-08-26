import {
  Activity,
  ChevronRight,
  CircleHelp,
  Globe2,
  RadioTower,
  TriangleAlert,
  Users,
} from "lucide-react";
import { memo, useMemo, useState } from "react";
import type { EdgeHistory, PathKind } from "../api/types";
import { pathLabel } from "../lib/format";
import { IdentityBadge } from "../lib/identity";
import { buildPathTimeline, pathColor } from "./historyMath";

interface Props {
  history: EdgeHistory;
}

export const PathTimeline = memo(function PathTimeline({ history }: Props) {
  const items = useMemo(() => buildPathTimeline(history), [history]);
  const [selectedID, setSelectedID] = useState("");
  const selected =
    items.find((item) => item.id === selectedID) ??
    items.find((item) => !item.anchored && item.observations.length > 0) ??
    items[0];
  const nodeByID = useMemo(
    () =>
      new Map([
        [history.source.id, history.source],
        [history.target.id, history.target],
      ]),
    [history.source, history.target],
  );

  return (
    <section className="history-section path-timeline-section">
      <h2>Path timeline</h2>
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
                onClick={() => setSelectedID(item.id)}
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
                  <strong>{pathLabel(item.path)}</strong>
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
      {selected ? (
        <div className="provenance-section">
          <h2>Observed by</h2>
          {selected.observations.length === 0 ? (
            <div className="provenance-empty">
              No observer provenance retained
            </div>
          ) : (
            <div
              className="provenance-table"
              role="table"
              aria-label="Observed by"
            >
              <div className="provenance-head" role="row">
                <span>Node</span>
                <span>Path</span>
                <span>First seen</span>
                <span>Last seen</span>
              </div>
              {selected.observations.map((observation, index) => {
                const node = nodeByID.get(observation.observerId);
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
                    <span className={`path-text ${observation.path.kind}`}>
                      {pathLabel(observation.path)}
                    </span>
                    <time dateTime={observation.collectedAt}>
                      {formatTimelineTime(observation.collectedAt, true)}
                    </time>
                    <time dateTime={observation.receivedAt}>
                      {formatTimelineTime(observation.receivedAt, true)}
                    </time>
                    {observation.relaySession ? (
                      <div className="history-relay-provenance">
                        <span>
                          Relay{" "}
                          {observation.path.peerRelayStableNodeId ?? "unknown"}
                        </span>
                        <span>VNI {observation.relaySession.vni}</span>
                        <span>
                          Session{" "}
                          <code>{observation.relaySession.sessionId}</code>
                        </span>
                        <IdentityBadge
                          status={observation.relaySession.sourceIdentityStatus}
                          compact
                        />
                        <IdentityBadge
                          status={observation.relaySession.targetIdentityStatus}
                          compact
                        />
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      ) : null}
    </section>
  );
});

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

function formatRelativeBoundary(value: string, from: string) {
  if (value === from) return "window start";
  return "path change";
}
