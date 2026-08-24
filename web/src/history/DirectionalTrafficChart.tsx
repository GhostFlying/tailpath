import { memo, useMemo, useState } from "react";
import type { EdgeHistory } from "../api/types";
import { formatRate } from "../lib/format";
import { trafficGeometry, trafficPointAtX } from "./historyMath";

interface Props {
  history: EdgeHistory;
}

export const DirectionalTrafficChart = memo(function DirectionalTrafficChart({
  history,
}: Props) {
  const geometry = useMemo(
    () =>
      trafficGeometry(
        history.traffic,
        history.bucketDurationMs,
        history.from,
        history.to,
      ),
    [history.bucketDurationMs, history.from, history.to, history.traffic],
  );
  const [hovered, setHovered] = useState<number | null>(null);
  const point = hovered === null ? null : geometry.points[hovered];
  const ticks = chartTicks(history.from, history.to);

  if (geometry.points.length === 0) {
    return (
      <section className="history-section traffic-chart-section">
        <h2>Traffic over time</h2>
        <div className="history-chart-empty">No traffic in this window</div>
      </section>
    );
  }

  return (
    <section className="history-section traffic-chart-section">
      <h2>Traffic over time</h2>
      <div className="traffic-chart-frame">
        <div className="traffic-y-labels" aria-hidden="true">
          <span>{formatRate(geometry.maxRate)}</span>
          <span>0 B/s</span>
          <span>{formatRate(geometry.maxRate)}</span>
        </div>
        <div className="traffic-svg-wrap">
          <svg
            className="traffic-chart"
            viewBox="0 0 900 260"
            preserveAspectRatio="none"
            aria-label={`${history.source.label} to ${history.target.label} above zero; ${history.target.label} to ${history.source.label} below zero`}
            onPointerLeave={() => setHovered(null)}
            onPointerMove={(event) => {
              const bounds = event.currentTarget.getBoundingClientRect();
              const x = Math.max(
                0,
                Math.min(bounds.width, event.clientX - bounds.left),
              );
              setHovered(
                trafficPointAtX(geometry.points, (x / bounds.width) * 900),
              );
            }}
          >
            {[0, 225, 450, 675, 900].map((x) => (
              <line
                key={`x-${x}`}
                x1={x}
                y1="0"
                x2={x}
                y2="260"
                className="chart-grid"
              />
            ))}
            {[18, 65, 130, 195, 242].map((y) => (
              <line
                key={`y-${y}`}
                x1="0"
                y1={y}
                x2="900"
                y2={y}
                className={y === 130 ? "chart-zero" : "chart-grid"}
              />
            ))}
            <path d={geometry.aArea} className="traffic-area traffic-area-a" />
            <path d={geometry.bArea} className="traffic-area traffic-area-b" />
            <path d={geometry.aLine} className="traffic-line traffic-line-a" />
            <path d={geometry.bLine} className="traffic-line traffic-line-b" />
            {point ? (
              <>
                <line
                  x1={point.x}
                  y1="0"
                  x2={point.x}
                  y2="260"
                  className="chart-cursor"
                />
                <circle
                  cx={point.x}
                  cy={point.aY}
                  r="5"
                  className="chart-point chart-point-a"
                />
                <circle
                  cx={point.x}
                  cy={point.bY}
                  r="5"
                  className="chart-point chart-point-b"
                />
              </>
            ) : null}
          </svg>
          {point ? (
            <div
              className="traffic-tooltip"
              style={{ left: `${(point.x / 900) * 100}%` }}
              role="status"
            >
              <time>{formatChartTime(point.at)}</time>
              <span className="a-direction">
                {history.source.label} → {history.target.label}
                <strong>{formatRate(point.aToBRate)}</strong>
              </span>
              <span className="b-direction">
                {history.target.label} → {history.source.label}
                <strong>{formatRate(point.bToARate)}</strong>
              </span>
            </div>
          ) : null}
          <div className="traffic-x-labels" aria-hidden="true">
            {ticks.map((tick) => (
              <span key={tick.toISOString()}>
                {formatChartTime(tick.toISOString())}
              </span>
            ))}
          </div>
        </div>
      </div>
      <div className="traffic-direction-legend">
        <span className="a-direction">
          <i /> {history.source.label} to {history.target.label}
        </span>
        <span className="b-direction">
          <i /> {history.target.label} to {history.source.label}
        </span>
      </div>
    </section>
  );
});

function chartTicks(from: string, to: string) {
  const start = new Date(from).getTime();
  const end = new Date(to).getTime();
  return Array.from(
    { length: 5 },
    (_, index) => new Date(start + ((end - start) * index) / 4),
  );
}

function formatChartTime(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(value));
}
