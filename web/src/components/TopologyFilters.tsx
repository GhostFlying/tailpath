import { Activity, Search, TriangleAlert } from "lucide-react";
import { useRef, useState } from "react";
import type { PathKind } from "../api/types";
import type { PathFilter } from "../lib/graph";

export const paths: { value: PathFilter; label: string }[] = [
  { value: "all", label: "All paths" },
  { value: "direct", label: "Direct" },
  { value: "derp", label: "DERP" },
  { value: "peer_relay", label: "Peer Relay" },
  { value: "unknown", label: "Unknown" },
];

interface Props {
  pathFilter: PathFilter;
  onPathFilterChange: (path: PathFilter) => void;
  query: string;
  onQueryChange: (query: string) => void;
  showRecent: boolean;
  onShowRecentChange: (showRecent: boolean) => void;
  counts: Record<PathKind, number>;
  edgeCount: number;
  liveRuntimes: number;
  totalRuntimes: number;
  skewedRuntimes: number;
}

export function TopologyFilters(props: Props) {
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false);
  const searchInput = useRef<HTMLInputElement>(null);

  function openMobileSearch() {
    setMobileSearchOpen(true);
    requestAnimationFrame(() => searchInput.current?.focus());
  }

  return (
    <aside className="filters" aria-label="Topology filters">
      <div className={`search-box ${mobileSearchOpen ? "mobile-open" : ""}`}>
        <button
          className="mobile-search-button"
          type="button"
          aria-label="Open node search"
          onClick={openMobileSearch}
        >
          <Search size={16} />
        </button>
        <Search className="desktop-search-icon" size={16} />
        <input
          ref={searchInput}
          value={props.query}
          onChange={(event) => props.onQueryChange(event.target.value)}
          onBlur={() => {
            if (!props.query) setMobileSearchOpen(false);
          }}
          placeholder="Find node"
          aria-label="Find node"
        />
      </div>

      <div className="mobile-path-select">
        <select
          value={props.pathFilter}
          onChange={(event) =>
            props.onPathFilterChange(event.target.value as PathFilter)
          }
          aria-label="Path filter"
        >
          {paths.map((path) => (
            <option key={path.value} value={path.value}>
              {path.label} ·{" "}
              {path.value === "all"
                ? props.edgeCount
                : props.counts[path.value]}
            </option>
          ))}
        </select>
      </div>

      <div className="filter-heading">
        <span>Path</span>
        <small>{props.edgeCount}</small>
      </div>
      <div className="path-filter">
        {paths.map((path) => (
          <button
            key={path.value}
            type="button"
            className={props.pathFilter === path.value ? "selected" : ""}
            onClick={() => props.onPathFilterChange(path.value)}
          >
            <i className={`path-glyph ${path.value}`} aria-hidden="true" />
            <span>{path.label}</span>
            <small>
              {path.value === "all"
                ? props.edgeCount
                : props.counts[path.value]}
            </small>
          </button>
        ))}
      </div>

      <div className="filter-heading activity-heading">
        <span>Activity</span>
      </div>
      <div className="recent-option">
        <span className="desktop-recent-label">Show recent</span>
        <span className="mobile-recent-label">Recent</span>
        <button
          type="button"
          role="switch"
          aria-checked={props.showRecent}
          aria-label="Show recent"
          onClick={() => props.onShowRecentChange(!props.showRecent)}
        >
          <span />
        </button>
      </div>

      <div className="runtime-summary">
        <Activity size={16} />
        <div>
          <strong>
            {props.liveRuntimes} of {props.totalRuntimes} runtimes reporting
          </strong>
          {props.skewedRuntimes > 0 ? (
            <span className="clock-warning">
              <TriangleAlert size={12} /> {props.skewedRuntimes} clock warning
            </span>
          ) : null}
        </div>
      </div>
    </aside>
  );
}
