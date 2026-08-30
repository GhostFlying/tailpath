import {
  Activity,
  ArrowDownLeft,
  ArrowUpRight,
  Clock3,
  Network,
  RadioTower,
  TriangleAlert,
  X,
} from "lucide-react";
import { useEffect, useState } from "react";
import { getEdgeHistory } from "../api/client";
import type {
  EdgeHistory,
  Topology,
  TopologyEdge,
  TopologyNode,
} from "../api/types";
import { MetadataConflictList } from "./MetadataConflictList";
import { formatAgo, formatRate, nodeLabel, pathLabel } from "../lib/format";
import { platformPresentation } from "../lib/platform";
import { IdentityBadge, unresolvedNodeLabel } from "../lib/identity";

interface Props {
  topology: Topology;
  edge: TopologyEdge | null;
  node: TopologyNode | null;
  onClose: () => void;
}

export function Inspector({ topology, edge, node, onClose }: Props) {
  const history = useEdgeHistory(edge?.id ?? null);
  if (!edge && !node) return null;
  return (
    <aside className="inspector" aria-label="Topology details">
      <button
        className="icon-button close-button"
        onClick={onClose}
        title="Close details"
        aria-label="Close details"
      >
        <X size={18} />
      </button>
      {edge ? (
        <EdgeDetails topology={topology} edge={edge} history={history} />
      ) : node ? (
        <NodeDetails topology={topology} node={node} />
      ) : null}
    </aside>
  );
}

function EdgeDetails({
  topology,
  edge,
  history,
}: {
  topology: Topology;
  edge: TopologyEdge;
  history: EdgeHistory | null;
}) {
  const source = topology.nodes.find((node) => node.id === edge.source);
  const target = topology.nodes.find((node) => node.id === edge.target);
  const relay = edge.path.peerRelayStableNodeId
    ? topology.nodes.find(
        (node) => node.stableNodeId === edge.path.peerRelayStableNodeId,
      )
    : undefined;
  return (
    <>
      <p className="panel-kicker">Traffic relationship</p>
      <h2>
        {source ? nodeLabel(source) : edge.source} <span>↔</span>{" "}
        {target ? nodeLabel(target) : edge.target}
      </h2>
      <div className={`path-banner ${edge.path.kind}`}>
        <Network size={17} />
        <strong>{pathLabel(edge.path)}</strong>
        <span className={`state-badge ${edge.state}`}>{edge.state}</span>
      </div>
      <dl className="details-list">
        {edge.path.directEndpoint ? (
          <Detail label="Endpoint" value={edge.path.directEndpoint} />
        ) : null}
        {edge.path.derpRegion ? (
          <Detail label="DERP region" value={edge.path.derpRegion} />
        ) : null}
        {edge.path.peerRelayStableNodeId ? (
          <Detail
            label="Relay node"
            value={relay ? nodeLabel(relay) : edge.path.peerRelayStableNodeId}
          />
        ) : null}
        {edge.path.peerRelayVni !== undefined ? (
          <Detail label="Relay VNI" value={String(edge.path.peerRelayVni)} />
        ) : null}
        <Detail
          label="Last active"
          value={formatAgo(edge.lastActive)}
          icon={<Clock3 size={15} />}
        />
      </dl>
      <section className="traffic-section">
        <h3>Current traffic</h3>
        <div className="direction-row">
          <ArrowUpRight size={17} />
          <span>
            {source ? nodeLabel(source) : "A"} to{" "}
            {target ? nodeLabel(target) : "B"}
          </span>
          <strong>{formatRate(edge.aToBBytesPerSecond)}</strong>
        </div>
        <div className="direction-row">
          <ArrowDownLeft size={17} />
          <span>
            {target ? nodeLabel(target) : "B"} to{" "}
            {source ? nodeLabel(source) : "A"}
          </span>
          <strong>{formatRate(edge.bToABytesPerSecond)}</strong>
        </div>
      </section>
      <section className="evidence-section">
        <h3>Observed by</h3>
        {edge.observations.map((observation) => {
          const observer = topology.nodes.find(
            (candidate) => candidate.id === observation.observerId,
          );
          return (
            <div
              className="evidence-row"
              key={`${observation.observerId}:${observation.relaySession?.sessionId ?? "peer"}`}
            >
              {observation.clockSkewed ? (
                <TriangleAlert size={15} aria-label="Runtime clock skew" />
              ) : (
                <RadioTower size={15} />
              )}
              <span>
                {observer ? nodeLabel(observer) : observation.observerId}
              </span>
              <small>{pathLabel(observation.path)}</small>
              {observation.relaySession ? (
                <div className="relay-evidence-details">
                  <span>
                    Session <code>{observation.relaySession.sessionId}</code>
                  </span>
                  <span>VNI {observation.relaySession.vni}</span>
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
        {edge.conflicts?.length ? (
          <p className="conflict-note">
            Conflicting path evidence is preserved in this edge.
          </p>
        ) : null}
      </section>
      {history?.pathEvents.length ? (
        <section className="history-section">
          <h3>Recent paths</h3>
          {history.pathEvents
            .slice(-5)
            .reverse()
            .map((event, index) => (
              <div
                className="history-row"
                key={`${event.observedAt}-${event.path.kind}-${index}`}
              >
                <span>{pathLabel(event.path)}</span>
                <small>{event.observations.length} sources</small>
                <time dateTime={event.observedAt}>
                  {formatAgo(event.observedAt)}
                </time>
              </div>
            ))}
        </section>
      ) : null}
    </>
  );
}

function NodeDetails({
  topology,
  node,
}: {
  topology: Topology;
  node: TopologyNode;
}) {
  const runtimeView = topology.observers.find(
    (candidate) => candidate.id === node.id,
  );
  const status = node.observable
    ? node.online
      ? "online"
      : "offline"
    : "runtime unknown";
  const platform = platformPresentation(node.os);
  const PlatformIcon = platform.Icon;
  return (
    <>
      <p className="panel-kicker">Tailnet node</p>
      <h2>{unresolvedNodeLabel(node.identityStatus) ?? nodeLabel(node)}</h2>
      <IdentityBadge status={node.identityStatus} />
      <div className="node-status">
        <PlatformIcon size={17} />
        <span>{platform.label}</span>
        <span className={`state-badge ${node.online ? "active" : "recent"}`}>
          {status}
        </span>
      </div>
      <dl className="details-list">
        <Detail
          label="Telemetry"
          value={node.observable ? "Runtime telemetry" : "Peer only"}
          icon={<Activity size={15} />}
        />
        {node.stableNodeId ? (
          <Detail label="Stable node ID" value={node.stableNodeId} />
        ) : null}
        {node.dnsName ? (
          <Detail label="MagicDNS" value={node.dnsName.replace(/\.$/, "")} />
        ) : null}
        {node.tailscaleIps?.length ? (
          <Detail label="Tailscale IP" value={node.tailscaleIps.join(", ")} />
        ) : null}
        {node.nodeKey ? <Detail label="Node key" value={node.nodeKey} /> : null}
        <Detail
          label="Last evidence"
          value={formatAgo(node.lastEvidenceAt)}
          icon={<Clock3 size={15} />}
        />
        {runtimeView?.clockSkewed ? (
          <Detail
            label="Collector clock"
            value={formatClockSkew(runtimeView.clockSkewMs)}
            icon={<TriangleAlert className="clock-warning" size={15} />}
          />
        ) : null}
      </dl>
      <MetadataConflictList conflicts={node.directory?.conflicts ?? []} />
    </>
  );
}

function useEdgeHistory(edgeID: string | null) {
  const [history, setHistory] = useState<EdgeHistory | null>(null);
  useEffect(() => {
    setHistory(null);
    if (!edgeID) return;
    const controller = new AbortController();
    void getEdgeHistory(edgeID, controller.signal)
      .then(setHistory)
      .catch(() => {
        if (!controller.signal.aborted) setHistory(null);
      });
    return () => controller.abort();
  }, [edgeID]);
  return history;
}

function formatClockSkew(milliseconds: number) {
  const seconds = Math.round(Math.abs(milliseconds) / 1000);
  return `${seconds}s ${milliseconds >= 0 ? "ahead" : "behind"}`;
}

function Detail({
  label,
  value,
  icon,
}: {
  label: string;
  value: string;
  icon?: React.ReactNode;
}) {
  return (
    <div>
      <dt>
        {icon}
        {label}
      </dt>
      <dd>{value}</dd>
    </div>
  );
}
