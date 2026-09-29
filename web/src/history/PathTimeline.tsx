import {
  Activity,
  ArrowRight,
  ChevronRight,
  CircleHelp,
  Globe2,
  RadioTower,
  TriangleAlert,
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
import type {
  EdgeHistory,
  DirectionalPathState,
  HistoryNodeReference,
  PathCandidate,
  PathKind,
  PathObservation,
} from "../api/types";
import { pathLabel, unresolvedPeerRelayLabel } from "../lib/format";
import { identityPresentation } from "../lib/identity";
import {
  buildPathTimeline,
  buildDirectionalTimeline,
  compatibilityPathEvidenceKey,
  coalesceDirectionalTimeline,
  hasDirectionalHistory,
  pathColor,
  pathEvidenceKey,
  type PathTimelineItem,
  type DirectionalTimelineSegment,
} from "./historyMath";

const eventIndexPageSize = 100;

interface Props {
  history: EdgeHistory;
  mobile: boolean;
  loading: boolean;
  loaded: number;
  complete: boolean;
  onSelectTime: (at: string) => void;
}

export const PathTimeline = memo(function PathTimeline(props: Props) {
  return hasDirectionalHistory(props.history) ? (
    <DirectionalPathTimeline {...props} />
  ) : (
    <LegacyPathTimeline {...props} />
  );
});

const LegacyPathTimeline = memo(function LegacyPathTimeline({
  history,
  mobile,
  loading,
  loaded,
  complete,
  onSelectTime,
}: Props) {
  const items = useMemo(() => buildPathTimeline(history), [history]);
  const [selectedID, setSelectedID] = useState("");
  const [sheetOpen, setSheetOpen] = useState(false);
  const [page, setPage] = useState(0);
  const closeSheet = useCallback(() => setSheetOpen(false), []);
  const selected =
    items.find((item) => item.id === selectedID) ??
    items.find((item) => !item.anchored && item.observations.length > 0) ??
    items[0];
  const nodes = useMemo(() => buildHistoryNodeMaps(history), [history]);
  const pageCount = Math.max(1, Math.ceil(items.length / eventIndexPageSize));
  const activePage = Math.min(page, pageCount - 1);
  const pageStart = activePage * eventIndexPageSize;
  const visibleItems = items.slice(pageStart, pageStart + eventIndexPageSize);

  useEffect(() => {
    if (!mobile) setSheetOpen(false);
  }, [mobile]);

  function select(item: PathTimelineItem) {
    setSelectedID(item.id);
    onSelectTime(item.from);
    if (mobile) setSheetOpen(true);
  }

  return (
    <section className="history-section path-timeline-section">
      <div className="history-section-heading">
        <h2>Path timeline</h2>
        <span className="legacy-timeline-status">
          <span>Newest first</span>
          <small>
            {loading
              ? `Loading · ${loaded} events`
              : complete
                ? `Complete · ${loaded} events`
                : "Legacy combined evidence"}
          </small>
        </span>
      </div>
      {items.length === 0 ? (
        <div className="history-chart-empty">
          No path evidence in this window
        </div>
      ) : (
        <>
          {pageCount > 1 ? (
            <div className="directional-event-index-toolbar legacy-timeline-pagination">
              <span>
                States {pageStart + 1}–{pageStart + visibleItems.length} of{" "}
                {items.length}
              </span>
              <button
                type="button"
                disabled={activePage === 0}
                onClick={() => setPage(activePage - 1)}
              >
                Newer states
              </button>
              <button
                type="button"
                disabled={activePage === pageCount - 1}
                onClick={() => setPage(activePage + 1)}
              >
                Older states
              </button>
            </div>
          ) : null}
          <div className="path-timeline" role="list" aria-label="Path timeline">
            {visibleItems.map((item, index) => {
              const Icon = pathIcon(item.path.kind);
              const active = item.id === selected?.id;
              const label = displayPathLabel(item.path, nodes.byStableID);
              const switching = item.pathState === "switching";
              const observerLabel = `${item.observations.length} observer${item.observations.length === 1 ? "" : "s"}`;
              return (
                <button
                  key={item.id}
                  type="button"
                  role="listitem"
                  aria-posinset={pageStart + index + 1}
                  aria-setsize={items.length}
                  className={active ? "selected" : ""}
                  style={
                    {
                      "--timeline-color": pathColor(item.path.kind),
                      "--timeline-grow": Math.max(1, item.durationMs),
                    } as React.CSSProperties
                  }
                  aria-pressed={active}
                  aria-label={`${formatTimelineTime(item.from)}, ${label}${switching ? ", switching" : ""}, ${formatDuration(item.durationMs)}, ${observerLabel}`}
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
                    <strong title={label}>{label}</strong>
                    <small>{formatDuration(item.durationMs)}</small>
                    {switching ? (
                      <span className="timeline-state">Switching</span>
                    ) : null}
                  </span>
                  <ChevronRight size={18} />
                </button>
              );
            })}
          </div>
        </>
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

const DirectionalPathTimeline = memo(function DirectionalPathTimeline({
  history,
  mobile,
  loading,
  loaded,
  complete,
  onSelectTime,
}: Props) {
  const segments = useMemo(() => buildDirectionalTimeline(history), [history]);
  const [selectedID, setSelectedID] = useState("");
  const [sheetOpen, setSheetOpen] = useState(false);
  const [eventIndexPageFromNewest, setEventIndexPageFromNewest] = useState(0);
  const canvasRef = useRef<HTMLDivElement>(null);
  const [canvasWidth, setCanvasWidth] = useState(0);
  const selected =
    segments.find((segment) => segment.id === selectedID) ?? segments.at(-1);
  const nodes = useMemo(() => buildHistoryNodeMaps(history), [history]);
  const renderBudget = Math.max(
    64,
    Math.min(240, Math.floor((canvasWidth || 720) / 4)),
  );
  const renderedSegments = useMemo(
    () => coalesceDirectionalTimeline(segments, renderBudget),
    [renderBudget, segments],
  );
  const eventIndexPageCount = Math.max(
    1,
    Math.ceil(segments.length / eventIndexPageSize),
  );
  const activeEventIndexPage = Math.max(
    0,
    eventIndexPageCount -
      1 -
      Math.min(eventIndexPageFromNewest, eventIndexPageCount - 1),
  );
  const eventIndexStart = Math.max(
    0,
    segments.length -
      (eventIndexPageCount - activeEventIndexPage) * eventIndexPageSize,
  );
  const eventIndexEnd =
    activeEventIndexPage === eventIndexPageCount - 1
      ? segments.length
      : Math.max(
          0,
          segments.length -
            (eventIndexPageCount - activeEventIndexPage - 1) *
              eventIndexPageSize,
        );
  const indexedSegments = segments.slice(eventIndexStart, eventIndexEnd);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const measure = () => setCanvasWidth(canvas.getBoundingClientRect().width);
    const observer = new ResizeObserver(measure);
    observer.observe(canvas);
    measure();
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!mobile) setSheetOpen(false);
  }, [mobile]);

  useEffect(() => {
    if (selected) onSelectTime(selected.from);
  }, [onSelectTime, selected]);

  function select(segment: DirectionalTimelineSegment) {
    const sourceID = segment.sourceSegmentId ?? segment.id;
    const sourceSegment =
      segments.find((candidate) => candidate.id === sourceID) ?? segment;
    setSelectedID(sourceID);
    const index = segments.findIndex((candidate) => candidate.id === sourceID);
    if (index >= 0) {
      setEventIndexPageFromNewest(
        Math.max(
          0,
          Math.ceil((segments.length - index) / eventIndexPageSize) - 1,
        ),
      );
    }
    onSelectTime(sourceSegment.from);
    if (mobile) setSheetOpen(true);
  }

  const shortFallbacks = segments.reduce((count, segment) => {
    const position = timelinePosition(segment, history);
    const short = (position.width / 100) * canvasWidth < 56;
    if (!short) return count;
    return (
      count +
      Number(Boolean(segment.aToB?.fallbackPath)) +
      Number(Boolean(segment.bToA?.fallbackPath))
    );
  }, 0);

  return (
    <section className="history-section path-timeline-section directional-history-section">
      <div className="history-section-heading">
        <h2>Directional path timeline</h2>
        <span>
          {loading
            ? `Loading path history · ${loaded} events`
            : complete
              ? `Complete · ${loaded} events`
              : `${loaded} embedded events`}
        </span>
      </div>
      {segments.length === 0 ? (
        <div className="history-chart-empty">
          No path evidence in this window
        </div>
      ) : (
        <>
          <div className="directional-timeline-layout">
            <div className="directional-lane-labels" aria-hidden="true">
              <DirectionLaneLabel
                from={history.source.label}
                to={history.target.label}
              />
              <span>DERP fallback</span>
              <DirectionLaneLabel
                from={history.target.label}
                to={history.source.label}
              />
              <span>DERP fallback</span>
            </div>
            <div className="directional-timeline-canvas" ref={canvasRef}>
              <div className="directional-time-axis" aria-hidden="true">
                {timelineTicks(history.from, history.to).map((tick) => (
                  <span key={tick.toISOString()}>
                    {formatTimelineTime(tick.toISOString())}
                  </span>
                ))}
              </div>
              <div className="directional-lane-grid" aria-hidden="true">
                {[0, 1, 2, 3].map((lane) => (
                  <i key={lane} />
                ))}
              </div>
              <div className="directional-visual-segments" aria-hidden="true">
                {renderedSegments.map((segment) => (
                  <DirectionalSegmentVisual
                    key={segment.id}
                    segment={segment}
                    history={history}
                    nodes={nodes}
                    canvasWidth={canvasWidth}
                  />
                ))}
              </div>
              <div
                className="directional-hit-segments"
                role="list"
                aria-label="Path timeline"
              >
                <span className="sr-only">
                  {segments.length} recorded path states.
                  {selected
                    ? ` Current: ${compactDirectionalLabel(selected, nodes)}.`
                    : ""}{" "}
                  Use the event index to inspect every retained event.
                </span>
                {renderedSegments.map((segment) => {
                  const position = timelinePosition(segment, history);
                  const active =
                    (segment.sourceSegmentId ?? segment.id) === selected?.id;
                  const widthPixels = (position.width / 100) * canvasWidth;
                  if (mobile && canvasWidth > 0 && widthPixels < 44) {
                    return (
                      <span
                        key={segment.id}
                        className="directional-short-hit"
                        style={{
                          left: `${position.left}%`,
                          width: `${position.width}%`,
                        }}
                        aria-hidden="true"
                      />
                    );
                  }
                  return (
                    <button
                      key={segment.id}
                      type="button"
                      role="listitem"
                      className={active ? "selected" : ""}
                      style={{
                        left: `${position.left}%`,
                        width: `${position.width}%`,
                      }}
                      aria-pressed={active}
                      aria-label={
                        segment.coalescedMixed
                          ? `${segment.coalescedCount ?? 2} recorded path states in this interval; select to inspect the latest exact state`
                          : directionalSegmentLabel(segment, history, nodes)
                      }
                      onClick={() => select(segment)}
                    />
                  );
                })}
              </div>
              {selected ? (
                <i
                  className="directional-time-cursor"
                  style={{
                    left: `${timelinePosition(selected, history).left}%`,
                  }}
                  aria-hidden="true"
                />
              ) : null}
            </div>
          </div>
          <div className="directional-timeline-footer">
            <span>Time flows left to right</span>
            {shortFallbacks ? (
              <span>+{shortFallbacks} short fallback windows</span>
            ) : null}
          </div>
        </>
      )}
      {selected && !mobile ? (
        <DirectionalSnapshotContent
          history={history}
          selected={selected}
          nodes={nodes}
        />
      ) : null}
      {selected && mobile && sheetOpen ? (
        <MobileDirectionalSheet
          history={history}
          selected={selected}
          nodes={nodes}
          onClose={() => setSheetOpen(false)}
        />
      ) : null}
      {segments.length ? (
        <details className="directional-event-index">
          <summary>{segments.length} recorded path states</summary>
          <div className="directional-event-index-toolbar">
            <span>
              States {eventIndexStart + 1}–
              {eventIndexStart + indexedSegments.length} of {segments.length}
            </span>
            <button
              type="button"
              disabled={activeEventIndexPage === 0}
              onClick={() =>
                setEventIndexPageFromNewest(eventIndexPageFromNewest + 1)
              }
            >
              Previous states
            </button>
            <button
              type="button"
              disabled={activeEventIndexPage === eventIndexPageCount - 1}
              onClick={() =>
                setEventIndexPageFromNewest(
                  Math.max(0, eventIndexPageFromNewest - 1),
                )
              }
            >
              Next states
            </button>
          </div>
          <div className="directional-event-index-list" role="list">
            {indexedSegments.map((segment, index) => (
              <button
                key={segment.id}
                type="button"
                role="listitem"
                aria-posinset={eventIndexStart + index + 1}
                aria-setsize={segments.length}
                aria-pressed={segment.id === selected?.id}
                onClick={() => select(segment)}
              >
                <time dateTime={segment.from}>
                  {formatTimelineDateTime(segment.from)}
                </time>
                <span>{compactDirectionalLabel(segment, nodes)}</span>
              </button>
            ))}
          </div>
        </details>
      ) : null}
    </section>
  );
});

function DirectionLaneLabel({ from, to }: { from: string; to: string }) {
  return (
    <strong title={`${from} to ${to}`}>
      <span>{from}</span>
      <ArrowRight size={13} />
      <span>{to}</span>
    </strong>
  );
}

function DirectionalSegmentVisual({
  segment,
  history,
  nodes,
  canvasWidth,
}: {
  segment: DirectionalTimelineSegment;
  history: EdgeHistory;
  nodes: HistoryNodeMaps;
  canvasWidth: number;
}) {
  const position = timelinePosition(segment, history);
  const showLabel =
    !segment.coalescedMixed && (position.width / 100) * canvasWidth >= 56;
  const style = {
    left: `${position.left}%`,
    width: `${position.width}%`,
  };
  const noDirections = !segment.aToB && !segment.bToA;
  if (segment.coalescedMixed) {
    return (
      <span className="directional-dense-segment" style={style}>
        {(position.width / 100) * canvasWidth >= 56
          ? `${segment.coalescedCount ?? 2} path states`
          : null}
      </span>
    );
  }
  if (noDirections && !segment.noEvidence) {
    return (
      <span className="directional-legacy-segment" style={style}>
        {showLabel ? "Legacy combined evidence" : null}
      </span>
    );
  }
  return (
    <>
      <DirectionalLaneSegment
        state={segment.aToB}
        lane="a-primary"
        style={style}
        showLabel={showLabel}
        nodes={nodes}
      />
      <DirectionalFallbackSegment
        state={segment.aToB}
        lane="a-fallback"
        style={style}
        showLabel={showLabel}
      />
      <DirectionalLaneSegment
        state={segment.bToA}
        lane="b-primary"
        style={style}
        showLabel={showLabel}
        nodes={nodes}
      />
      <DirectionalFallbackSegment
        state={segment.bToA}
        lane="b-fallback"
        style={style}
        showLabel={showLabel}
      />
    </>
  );
}

function DirectionalLaneSegment({
  state,
  lane,
  style,
  showLabel,
  nodes,
}: {
  state?: DirectionalPathState;
  lane: "a-primary" | "b-primary";
  style: React.CSSProperties;
  showLabel: boolean;
  nodes: HistoryNodeMaps;
}) {
  const kind = state?.primaryPath.kind ?? "unknown";
  return (
    <span
      className={`directional-lane-segment ${lane} ${kind}`}
      style={
        { ...style, "--timeline-color": pathColor(kind) } as React.CSSProperties
      }
    >
      {showLabel
        ? state
          ? displayPathLabel(state.primaryPath, nodes.byStableID)
          : "Unknown"
        : null}
    </span>
  );
}

function DirectionalFallbackSegment({
  state,
  lane,
  style,
  showLabel,
}: {
  state?: DirectionalPathState;
  lane: "a-fallback" | "b-fallback";
  style: React.CSSProperties;
  showLabel: boolean;
}) {
  if (!state?.fallbackPath) return null;
  return (
    <span className={`directional-fallback-segment ${lane}`} style={style}>
      {showLabel
        ? `DERP fallback${state.fallbackPath.derpRegion ? ` · ${state.fallbackPath.derpRegion}` : ""}`
        : null}
    </span>
  );
}

function DirectionalSnapshotContent({
  history,
  selected,
  nodes,
  onClose,
  closeButtonRef,
}: {
  history: EdgeHistory;
  selected: DirectionalTimelineSegment;
  nodes: HistoryNodeMaps;
  onClose?: () => void;
  closeButtonRef?: RefObject<HTMLButtonElement | null>;
}) {
  const directions = [
    {
      label: `${history.source.label} → ${history.target.label}`,
      state: selected.aToB,
    },
    {
      label: `${history.target.label} → ${history.source.label}`,
      state: selected.bToA,
    },
  ];
  const observations = selected.event.observations;
  return (
    <div className="provenance-section directional-snapshot">
      <header className="provenance-title">
        <div>
          <span>Effective paths at</span>
          <strong>{formatTimelineDateTime(selected.from)}</strong>
        </div>
        <span className="directional-snapshot-state">
          {snapshotIsAsymmetric(selected)
            ? "Asymmetric paths"
            : "Directional snapshot"}
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
      {!selected.aToB && !selected.bToA && !selected.noEvidence ? (
        <div className="legacy-combined-callout">
          <strong>Legacy combined evidence</strong>
          <span>
            This record predates directional storage. Tailpath cannot recreate
            which endpoint selected {pathLabel(selected.event.path)}.
          </span>
        </div>
      ) : selected.noEvidence ? (
        <div className="legacy-combined-callout no-evidence">
          <strong>Unknown / No fresh observation</strong>
          <span>No retained path state covers this part of the window.</span>
        </div>
      ) : (
        <div
          className="directional-state-table"
          role="table"
          aria-label="Directional path state"
        >
          <div role="row" className="directional-state-head">
            <span>Direction</span>
            <span>Primary path</span>
            <span>Fallback</span>
            <span>Evidence</span>
          </div>
          {directions.map(({ label, state }) => (
            <div role="row" className="directional-state-row" key={label}>
              <strong>{label}</strong>
              <span>
                {state
                  ? displayPathLabel(state.primaryPath, nodes.byStableID)
                  : "Unknown"}
                {state ? (
                  <small>{directionPathMetadata(state.primaryPath)}</small>
                ) : (
                  <small>No fresh observation</small>
                )}
              </span>
              <span>
                {directionFallbackLabel(state)}
                {state?.inferenceRule ? (
                  <code>{state.inferenceRule}</code>
                ) : null}
              </span>
              <DirectionEvidence state={state} nodes={nodes} />
            </div>
          ))}
        </div>
      )}
      <h2>Evidence retained at this event</h2>
      {observations.length ? (
        <div className="directional-history-evidence">
          {observations.map((observation, index) => (
            <div key={`${observation.observerId}:${index}`}>
              <strong>
                {nodes.byID.get(observation.observerId)?.label ??
                  observation.observerId}
              </strong>
              <span>
                {observation.relaySession ? "Relay identity" : "Endpoint path"}
              </span>
              <span>
                {displayPathLabel(observation.path, nodes.byStableID)}
              </span>
              <span className="directional-observation-times">
                <small>
                  Collected{" "}
                  <time dateTime={observation.collectedAt}>
                    {formatTimelineTime(observation.collectedAt, true)}
                  </time>
                </small>
                <small>
                  Received{" "}
                  <time dateTime={observation.receivedAt}>
                    {formatTimelineTime(observation.receivedAt, true)}
                  </time>
                </small>
                {observation.clockSkewed ? (
                  <small className="direction-clock-warning">
                    <TriangleAlert size={12} aria-hidden="true" /> Collector
                    clock warning
                  </small>
                ) : null}
              </span>
              {observation.relaySession ? (
                <RelaySessionDetails
                  history={history}
                  observation={observation}
                  nodes={nodes}
                />
              ) : null}
            </div>
          ))}
        </div>
      ) : (
        <div className="provenance-empty">No observer provenance retained</div>
      )}
    </div>
  );
}

export function directionFallbackLabel(
  state: DirectionalPathState | undefined,
): string {
  if (!state) return "Unknown";
  return state.fallbackPath ? pathLabel(state.fallbackPath) : "None";
}

function MobileDirectionalSheet({
  history,
  selected,
  nodes,
  onClose,
}: {
  history: EdgeHistory;
  selected: DirectionalTimelineSegment;
  nodes: HistoryNodeMaps;
  onClose: () => void;
}) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  useDialogFocus(dialogRef, closeButtonRef, onClose);
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
        <DirectionalSnapshotContent
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

function useDialogFocus(
  dialogRef: RefObject<HTMLDivElement | null>,
  closeButtonRef: RefObject<HTMLButtonElement | null>,
  onClose: () => void,
) {
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
      if (!focusable.length) return;
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
  }, [closeButtonRef, dialogRef, onClose]);
}

function timelinePosition(
  segment: DirectionalTimelineSegment,
  history: EdgeHistory,
) {
  const start = new Date(history.from).getTime();
  const end = new Date(history.to).getTime();
  const from = new Date(segment.from).getTime();
  const to = new Date(segment.to).getTime();
  const duration = Math.max(1, end - start);
  return {
    left: ((from - start) / duration) * 100,
    width: Math.max(0, ((to - from) / duration) * 100),
  };
}

function timelineTicks(from: string, to: string) {
  const start = new Date(from).getTime();
  const end = new Date(to).getTime();
  return Array.from(
    { length: 5 },
    (_, index) => new Date(start + ((end - start) * index) / 4),
  );
}

function directionalSegmentLabel(
  segment: DirectionalTimelineSegment,
  history: EdgeHistory,
  nodes: HistoryNodeMaps,
) {
  return `${formatTimelineDateTime(segment.from)}; ${history.source.label} to ${history.target.label}: ${directionStateLabel(segment.aToB, nodes)}; ${history.target.label} to ${history.source.label}: ${directionStateLabel(segment.bToA, nodes)}`;
}

function directionStateLabel(
  state: DirectionalPathState | undefined,
  nodes: HistoryNodeMaps,
) {
  if (!state) return "Unknown, no fresh observation";
  const fallback = state.fallbackPath
    ? ` with ${pathLabel(state.fallbackPath)} fallback`
    : "";
  return `${displayPathLabel(state.primaryPath, nodes.byStableID)}${fallback}, ${state.evidence}`;
}

function compactDirectionalLabel(
  segment: DirectionalTimelineSegment,
  nodes: HistoryNodeMaps,
) {
  if (segment.noEvidence) return "Unknown / No fresh observation";
  if (!segment.aToB && !segment.bToA) return "Legacy combined evidence";
  return `${directionStateLabel(segment.aToB, nodes)} / ${directionStateLabel(segment.bToA, nodes)}`;
}

function snapshotIsAsymmetric(segment: DirectionalTimelineSegment) {
  if (!segment.aToB || !segment.bToA) return false;
  return (
    directionalStateKey(segment.aToB) !== directionalStateKey(segment.bToA)
  );
}

function directionalStateKey(state: DirectionalPathState) {
  return `${pathEvidenceKey(state.primaryPath)}|${state.fallbackPath ? pathEvidenceKey(state.fallbackPath) : "none"}`;
}

function directionPathMetadata(path: PathObservation) {
  return [
    path.peerRelayVni !== undefined ? `VNI ${path.peerRelayVni}` : undefined,
    path.peerRelayEndpoint,
    peerRelayResolutionLabel(path.peerRelayResolution),
    path.directEndpoint,
  ]
    .filter(Boolean)
    .join(" · ");
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

function DirectionEvidence({
  state,
  nodes,
}: {
  state?: DirectionalPathState;
  nodes: HistoryNodeMaps;
}) {
  if (!state) {
    return (
      <span className="directional-state-evidence">
        <span className="history-evidence-badge unknown">Unknown</span>
        <small>No fresh observation</small>
      </span>
    );
  }
  const observer = nodes.byID.get(state.observerId)?.label ?? state.observerId;
  return (
    <span className="directional-state-evidence">
      <span className={`history-evidence-badge ${state.evidence}`}>
        {capitalize(state.evidence)}
      </span>
      <small>Observer {observer}</small>
      <small>
        Collected{" "}
        <time dateTime={state.collectedAt}>
          {formatTimelineTime(state.collectedAt, true)}
        </time>
      </small>
      <small>
        Received{" "}
        <time dateTime={state.receivedAt}>
          {formatTimelineTime(state.receivedAt, true)}
        </time>
      </small>
      {state.clockSkewed ? (
        <small className="direction-clock-warning">
          <TriangleAlert size={12} aria-hidden="true" /> Collector clock warning
        </small>
      ) : null}
    </span>
  );
}

function capitalize(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

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
      {selected.pathCandidates?.length ? (
        <HistoryPathCandidates
          candidates={selected.pathCandidates}
          switching={selected.pathState === "switching"}
          nodes={nodes}
        />
      ) : null}
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
              compatibilityPathEvidenceKey(observation.path) ===
              compatibilityPathEvidenceKey(selected.path);
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

function HistoryPathCandidates({
  candidates,
  switching,
  nodes,
}: {
  candidates: PathCandidate[];
  switching: boolean;
  nodes: HistoryNodeMaps;
}) {
  return (
    <section
      className="history-path-candidates"
      aria-label="Historical path candidates"
    >
      <header>
        <div>
          <h2>Path candidates</h2>
          <span>{candidates.length} preserved at this event</span>
        </div>
        {switching ? (
          <strong className="history-switching-state">Switching</strong>
        ) : null}
      </header>
      <div className="history-candidate-list">
        {candidates.map((candidate, index) => {
          const path = candidate.path;
          const relay = path.peerRelayStableNodeId
            ? nodes.byStableID.get(path.peerRelayStableNodeId)
            : undefined;
          const label =
            relay?.label ?? path.peerRelayStableNodeId ?? "Unresolved relay";
          return (
            <article
              key={`${path.peerRelayStableNodeId ?? path.peerRelayEndpoint ?? "unknown"}:${index}`}
              className={path.peerRelayStableNodeId ? "identified" : "pending"}
            >
              <span className="history-candidate-symbol" aria-hidden="true">
                {path.peerRelayStableNodeId
                  ? label.slice(0, 1).toUpperCase()
                  : "?"}
              </span>
              <div>
                <strong>{label}</strong>
                <small>
                  {path.peerRelayVni !== undefined
                    ? `VNI ${path.peerRelayVni}`
                    : "VNI unavailable"}
                  {` · ${candidate.observerCount} observer${candidate.observerCount === 1 ? "" : "s"}`}
                </small>
                {path.peerRelayEndpoint ? (
                  <code>{path.peerRelayEndpoint}</code>
                ) : null}
              </div>
              <div className="history-candidate-meta">
                <span>{historicalResolutionLabel(candidate)}</span>
                <time dateTime={candidate.lastObservedAt}>
                  {formatTimelineDateTime(candidate.lastObservedAt)}
                </time>
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
}

function historicalResolutionLabel(candidate: PathCandidate) {
  switch (candidate.path.peerRelayResolution) {
    case "relay_session":
      return "Relay session";
    case "tailscale_ip":
      return "Tailscale IP";
    case "endpoint_match":
      return "Matched by endpoint";
    default:
      return candidate.path.peerRelayStableNodeId
        ? "Identified"
        : "Identity pending";
  }
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
  if (path.kind !== "peer_relay") {
    return pathLabel(path);
  }
  if (!path.peerRelayStableNodeId) return unresolvedPeerRelayLabel(path);
  const relay = byStableID.get(path.peerRelayStableNodeId);
  return `${relay?.label ?? path.peerRelayStableNodeId} · Peer Relay`;
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
