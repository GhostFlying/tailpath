import {
  Activity,
  ArrowDownLeft,
  ArrowRight,
  ArrowUpRight,
  Clock3,
  Network,
  RefreshCcw,
  RadioTower,
  TriangleAlert,
  X,
} from "lucide-react";
import { useEffect, useState } from "react";
import { getEdgeHistory } from "../api/client";
import type {
  DirectionalPathState,
  EdgeHistory,
  PathEvent,
  PathCandidate,
  PathObservation,
  Topology,
  TopologyEdge,
  TopologyNode,
} from "../api/types";
import { MetadataConflictList } from "./MetadataConflictList";
import { formatAgo, formatRate, nodeLabel, pathLabel } from "../lib/format";
import {
  edgeDirections,
  edgeIsAsymmetric,
  peerRelayCandidateKey,
  peerRelayCandidates,
} from "../lib/graph";
import { platformPresentation } from "../lib/platform";
import { IdentityBadge, unresolvedNodeLabel } from "../lib/identity";
import { pathIdentityKey } from "../lib/pathIdentity";

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
  const candidates = peerRelayCandidates(edge);
  const switching = edge.pathState === "switching" || candidates.length > 1;
  const firstCandidateKey = candidates[0]
    ? peerRelayCandidateKey(candidates[0].path)
    : "";
  const [selectedCandidate, setSelectedCandidate] = useState(firstCandidateKey);
  useEffect(() => {
    setSelectedCandidate(firstCandidateKey);
  }, [edge.id, edge.pathState, firstCandidateKey]);
  if (edgeDirections(edge).length) {
    return (
      <DirectionalEdgeDetails
        topology={topology}
        edge={edge}
        history={history}
      />
    );
  }
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
        {switching ? (
          <span className="path-state switching">
            <RefreshCcw size={12} /> Switching
          </span>
        ) : null}
        <span className={`state-badge ${edge.state}`}>{edge.state}</span>
      </div>
      {candidates.length ? (
        <section className="candidate-section">
          <div className="candidate-heading">
            <h3>Path candidates</h3>
            <span>{candidates.length} fresh</span>
          </div>
          <div className="candidate-list">
            {candidates.map((candidate) => {
              const key = peerRelayCandidateKey(candidate.path);
              return (
                <RelayCandidate
                  key={key}
                  topology={topology}
                  candidate={candidate}
                  selected={selectedCandidate === key}
                  onSelect={() => setSelectedCandidate(key)}
                />
              );
            })}
          </div>
          {switching ? (
            <p className="switching-explanation">
              <RefreshCcw size={14} />
              <span>
                <strong>Why Switching?</strong> Multiple relay candidates are
                still fresh. Tailpath keeps each visible until its evidence
                expires or observers converge.
              </span>
            </p>
          ) : null}
        </section>
      ) : null}
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
        {edge.path.peerRelayEndpoint ? (
          <Detail label="Relay endpoint" value={edge.path.peerRelayEndpoint} />
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
        {switching ? (
          <p className="traffic-candidate-note">
            Traffic belongs to this relationship and is not duplicated across
            relay candidates.
          </p>
        ) : null}
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
        {edge.conflicts?.some((path) => path.kind !== "peer_relay") ? (
          <p className="conflict-note">
            Conflicting path evidence is preserved in this edge.
          </p>
        ) : null}
      </section>
      <RecentPaths history={history} />
    </>
  );
}

function DirectionalEdgeDetails({
  topology,
  edge,
  history,
}: {
  topology: Topology;
  edge: TopologyEdge;
  history: EdgeHistory | null;
}) {
  const directions = edgeDirections(edge);
  const source = topology.nodes.find((node) => node.id === edge.source);
  const target = topology.nodes.find((node) => node.id === edge.target);
  const endpointEvidence = edge.observations.filter(
    (observation) => !observation.relaySession,
  );
  const relayEvidence = edge.observations.filter((observation) =>
    Boolean(observation.relaySession),
  );
  const fallbackExplanationText = fallbackExplanation(directions);
  const slots = [
    {
      from: edge.source,
      to: edge.target,
      state: directions.find(
        (direction) =>
          direction.fromNodeId === edge.source &&
          direction.toNodeId === edge.target,
      ),
      rate: edge.aToBBytesPerSecond,
    },
    {
      from: edge.target,
      to: edge.source,
      state: directions.find(
        (direction) =>
          direction.fromNodeId === edge.target &&
          direction.toNodeId === edge.source,
      ),
      rate: edge.bToABytesPerSecond,
    },
  ];
  return (
    <>
      <p className="panel-kicker">Traffic relationship</p>
      <h2>
        {source ? nodeLabel(source) : edge.source} <span>↔</span>{" "}
        {target ? nodeLabel(target) : edge.target}
      </h2>
      <div className="directional-status">
        <span className="directional-status-icon" aria-hidden="true">
          ⇄
        </span>
        <span>
          <strong>
            {directions.length === 1
              ? "Partial path evidence"
              : edgeIsAsymmetric(edge)
                ? "Asymmetric paths"
                : "Same path both directions"}
          </strong>
          <small>
            {directions.length === 1
              ? "One direction has fresh path evidence; the reverse remains unknown."
              : edgeIsAsymmetric(edge)
                ? "Each endpoint currently reports a different route."
                : "Both endpoints report the same logical route."}
          </small>
        </span>
        <span className={`state-badge ${edge.state}`}>{edge.state}</span>
      </div>

      <section
        className="direction-path-section"
        aria-label="Directional paths"
      >
        {slots.map((slot) => (
          <DirectionalPathCard
            key={`${slot.from}:${slot.to}`}
            topology={topology}
            from={slot.from}
            to={slot.to}
            state={slot.state}
            rate={slot.rate}
          />
        ))}
      </section>

      {fallbackExplanationText ? (
        <p className="fallback-explanation">{fallbackExplanationText}</p>
      ) : null}

      <dl className="details-list directional-summary">
        <Detail
          label="Last active"
          value={formatAgo(edge.lastActive)}
          icon={<Clock3 size={15} />}
        />
      </dl>

      <EvidenceGroup
        title="Endpoint path evidence"
        empty="No endpoint provenance is available."
        topology={topology}
        observations={endpointEvidence}
      />
      <EvidenceGroup
        title="Relay identity evidence"
        empty="No relay session identity evidence is available."
        topology={topology}
        observations={relayEvidence}
      />
      <RecentPaths history={history} />
    </>
  );
}

export function fallbackExplanation(
  directions: DirectionalPathState[],
): string | null {
  const fallbackEvidence = new Set(
    directions
      .filter((direction) => direction.fallbackPath)
      .map((direction) => direction.evidence),
  );
  if (!fallbackEvidence.size) return null;

  const evidenceDescription =
    fallbackEvidence.size > 1
      ? "DERP fallback evidence includes observed and inferred parallel routes."
      : fallbackEvidence.has("inferred")
        ? "DERP fallback is an inferred parallel route."
        : "DERP fallback is an observed parallel route.";
  return `${evidenceDescription} Relationship traffic is counted once; the fallback line does not represent extra application traffic.`;
}

function RecentPaths({ history }: { history: EdgeHistory | null }) {
  if (!history?.pathEvents.length) return null;
  return (
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
            <span>{pathEventLabel(event)}</span>
            <small>{event.observations.length} sources</small>
            <time dateTime={event.observedAt}>
              {formatAgo(event.observedAt)}
            </time>
          </div>
        ))}
    </section>
  );
}

export function pathEventLabel(event: PathEvent) {
  const directions = event.directions ?? [];
  if (directions.length === 1) {
    const direction = directions[0];
    const fallback = direction.fallbackPath ? " + DERP fallback" : "";
    return `Partial · ${pathLabel(direction.primaryPath)}${fallback}`;
  }
  if (
    directions.length === 2 &&
    directionalStateKey(directions[0]) !== directionalStateKey(directions[1])
  ) {
    return "Asymmetric paths";
  }
  if (directions.length === 2) {
    const primary = pathLabel(directions[0].primaryPath);
    return directions[0].fallbackPath ? `${primary} + DERP fallback` : primary;
  }
  return event.pathState === "switching"
    ? "Peer Relay · Switching"
    : pathLabel(event.path);
}

function directionalStateKey(state: DirectionalPathState) {
  return `${pathIdentityKey(state.primaryPath)}|${state.fallbackPath ? pathIdentityKey(state.fallbackPath) : "none"}`;
}

function DirectionalPathCard({
  topology,
  from,
  to,
  state,
  rate,
}: {
  topology: Topology;
  from: string;
  to: string;
  state?: DirectionalPathState;
  rate: number;
}) {
  const fromNode = topology.nodes.find((node) => node.id === from);
  const toNode = topology.nodes.find((node) => node.id === to);
  return (
    <article
      className={`direction-path-card ${state ? state.primaryPath.kind : "unknown"}`}
    >
      <header>
        <span className="direction-endpoints">
          <strong>{fromNode ? nodeLabel(fromNode) : from}</strong>
          <ArrowRight size={15} aria-hidden="true" />
          <strong>{toNode ? nodeLabel(toNode) : to}</strong>
        </span>
        <b>{formatRate(rate)}</b>
      </header>
      {state ? (
        <>
          <div className="direction-primary">
            <span>Primary</span>
            <strong>{pathLabel(state.primaryPath)}</strong>
            <EvidenceBadge evidence={state.evidence} />
            <PathMetadata topology={topology} path={state.primaryPath} />
          </div>
          {state.fallbackPath ? (
            <div className="direction-fallback">
              <span>DERP fallback</span>
              <strong>{pathLabel(state.fallbackPath)}</strong>
              <EvidenceBadge evidence={state.evidence} />
              {state.inferenceRule ? <code>{state.inferenceRule}</code> : null}
            </div>
          ) : null}
          <small className="direction-observer">
            Reported by {nodeName(topology, state.observerId)} ·{" "}
            {formatAgo(state.receivedAt)}
          </small>
        </>
      ) : (
        <div className="direction-unknown">
          <strong>Unknown</strong>
          <span>No fresh observation from this endpoint.</span>
        </div>
      )}
    </article>
  );
}

function EvidenceBadge({
  evidence,
}: {
  evidence: DirectionalPathState["evidence"];
}) {
  return (
    <span className={`evidence-badge ${evidence}`}>{capitalize(evidence)}</span>
  );
}

function PathMetadata({
  topology,
  path,
}: {
  topology: Topology;
  path: PathObservation;
}) {
  const relay = path.peerRelayStableNodeId
    ? topology.nodes.find(
        (node) => node.stableNodeId === path.peerRelayStableNodeId,
      )
    : undefined;
  const values = [
    relay ? nodeLabel(relay) : path.peerRelayStableNodeId,
    path.peerRelayVni !== undefined ? `VNI ${path.peerRelayVni}` : undefined,
    path.peerRelayEndpoint,
    peerRelayResolutionLabel(path.peerRelayResolution),
    path.directEndpoint,
  ].filter(Boolean);
  return values.length ? <small>{values.join(" · ")}</small> : null;
}

function peerRelayResolutionLabel(
  resolution: PathObservation["peerRelayResolution"],
) {
  switch (resolution) {
    case "relay_session":
      return "Resolution: relay session";
    case "tailscale_ip":
      return "Resolution: Tailscale IP";
    case "endpoint_match":
      return "Resolution: endpoint match";
    default:
      return undefined;
  }
}

function EvidenceGroup({
  title,
  empty,
  topology,
  observations,
}: {
  title: string;
  empty: string;
  topology: Topology;
  observations: TopologyEdge["observations"];
}) {
  return (
    <section className="evidence-section">
      <h3>{title}</h3>
      {observations.length ? (
        observations.map((observation) => (
          <div
            className="evidence-row"
            key={`${observation.observerId}:${observation.relaySession?.sessionId ?? "peer"}`}
          >
            {observation.clockSkewed ? (
              <TriangleAlert size={15} aria-label="Runtime clock skew" />
            ) : (
              <RadioTower size={15} />
            )}
            <span>{nodeName(topology, observation.observerId)}</span>
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
        ))
      ) : (
        <p className="evidence-empty">{empty}</p>
      )}
    </section>
  );
}

function nodeName(topology: Topology, id: string) {
  const node = topology.nodes.find((candidate) => candidate.id === id);
  return node ? nodeLabel(node) : id;
}

function capitalize(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function RelayCandidate({
  topology,
  candidate,
  selected,
  onSelect,
}: {
  topology: Topology;
  candidate: PathCandidate;
  selected: boolean;
  onSelect: () => void;
}) {
  const stableID = candidate.path.peerRelayStableNodeId;
  const relay = stableID
    ? topology.nodes.find((node) => node.stableNodeId === stableID)
    : undefined;
  const label = relay ? nodeLabel(relay) : stableID || "Unresolved relay";
  const resolution = candidateResolution(candidate);
  return (
    <button
      type="button"
      className={`relay-candidate ${selected ? "selected" : ""}`}
      aria-expanded={selected}
      onClick={onSelect}
    >
      <span
        className={`relay-candidate-icon ${stableID ? "known" : "pending"}`}
      >
        {stableID ? label.slice(0, 1).toUpperCase() : "?"}
      </span>
      <span className="relay-candidate-copy">
        <strong>{label}</strong>
        <small>
          {candidate.path.peerRelayVni !== undefined
            ? `VNI ${candidate.path.peerRelayVni}`
            : "VNI unavailable"}
          {candidate.observerCount > 0
            ? ` · ${candidate.observerCount} observer${candidate.observerCount === 1 ? "" : "s"}`
            : ""}
        </small>
        <span className={`candidate-resolution ${resolution.kind}`}>
          {resolution.label}
        </span>
      </span>
      <time dateTime={candidate.lastObservedAt}>
        {formatAgo(candidate.lastObservedAt)}
      </time>
      {selected ? (
        <span className="relay-candidate-detail">
          {candidate.path.peerRelayEndpoint ? (
            <span>
              Endpoint <code>{candidate.path.peerRelayEndpoint}</code>
            </span>
          ) : null}
          <span>{resolution.explanation}</span>
        </span>
      ) : null}
    </button>
  );
}

function candidateResolution(candidate: PathCandidate) {
  switch (candidate.path.peerRelayResolution) {
    case "relay_session":
    case "tailscale_ip":
      return {
        kind: "verified",
        label: "Identity verified",
        explanation:
          "Authenticated relay evidence identifies this Tailnet node.",
      };
    case "endpoint_match":
      return {
        kind: "matched",
        label: "Matched by endpoints",
        explanation:
          "A fresh public IP uniquely matches this relay candidate. The normal Tailscale and relay service ports may differ.",
      };
    default:
      return candidate.path.peerRelayStableNodeId
        ? {
            kind: "verified",
            label: "Identity verified",
            explanation: "Relay evidence includes a stable Tailnet identity.",
          }
        : {
            kind: "pending",
            label: "Identity pending",
            explanation:
              "Observers agree on the relay endpoint, but no unique stable identity is available.",
          };
  }
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
