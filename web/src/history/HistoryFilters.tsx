import { Search } from "lucide-react";
import { useDeferredValue, useEffect, useMemo, useState } from "react";
import type {
  HistoryNodeReference,
  HistoryWindow,
  PathKind,
} from "../api/types";
import type { HistoryURLState } from "./historyUrl";

const windows: HistoryWindow[] = ["15m", "1h", "6h", "24h", "7d"];

interface Props {
  state: HistoryURLState;
  nodes: HistoryNodeReference[];
  onChange: (update: Partial<HistoryURLState>) => void;
}

export function HistoryFilters({ state, nodes, onChange }: Props) {
  const selectedNode = useMemo(
    () => nodes.find((node) => node.id === state.nodeId),
    [nodes, state.nodeId],
  );
  const [nodeQuery, setNodeQuery] = useState(selectedNode?.label ?? "");
  const [focused, setFocused] = useState(false);
  const deferredQuery = useDeferredValue(nodeQuery.trim().toLowerCase());
  useEffect(() => {
    setNodeQuery(selectedNode?.label ?? "");
  }, [selectedNode?.label]);
  const suggestions = useMemo(() => {
    if (!deferredQuery) return nodes.slice(0, 8);
    return nodes
      .filter((node) =>
        `${node.label} ${node.hostname ?? ""} ${node.dnsName ?? ""}`
          .toLowerCase()
          .includes(deferredQuery),
      )
      .slice(0, 8);
  }, [deferredQuery, nodes]);

  function updateNodeQuery(value: string) {
    setNodeQuery(value);
    if (!value && state.nodeId) onChange({ nodeId: "" });
  }

  function selectNode(node: HistoryNodeReference) {
    setNodeQuery(node.label);
    setFocused(false);
    onChange({ nodeId: node.id });
  }

  return (
    <section className="history-filters" aria-label="History filters">
      <div className="history-window-segments" aria-label="History window">
        {windows.map((window) => (
          <button
            key={window}
            type="button"
            className={state.window === window ? "selected" : ""}
            aria-pressed={state.window === window}
            onClick={() => onChange({ window })}
          >
            {window}
          </button>
        ))}
      </div>
      <label className="history-mobile-window">
        <span className="sr-only">History window</span>
        <select
          aria-label="History window"
          value={state.window}
          onChange={(event) =>
            onChange({ window: event.target.value as HistoryWindow })
          }
        >
          {windows.map((window) => (
            <option key={window} value={window}>
              {window}
            </option>
          ))}
        </select>
      </label>
      <div className="history-node-filter">
        <label>
          <Search size={17} />
          <input
            aria-label="Find node"
            value={nodeQuery}
            placeholder="Find node"
            onChange={(event) => updateNodeQuery(event.target.value)}
            onFocus={() => setFocused(true)}
            onBlur={() => window.setTimeout(() => setFocused(false), 100)}
          />
        </label>
        {focused && suggestions.length > 0 ? (
          <div className="history-suggestions" role="listbox">
            {suggestions.map((node) => (
              <button
                key={node.id}
                type="button"
                role="option"
                aria-selected={node.id === state.nodeId}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => selectNode(node)}
              >
                <strong>{node.label}</strong>
                <span>{node.os || "Unknown platform"}</span>
              </button>
            ))}
          </div>
        ) : null}
      </div>
      <label className="history-path-filter">
        <span className="sr-only">Path seen</span>
        <select
          aria-label="Path seen"
          value={state.path}
          onChange={(event) =>
            onChange({ path: event.target.value as PathKind | "" })
          }
        >
          <option value="">All paths</option>
          <option value="direct">Direct</option>
          <option value="derp">DERP</option>
          <option value="peer_relay">Peer Relay</option>
          <option value="unknown">Unknown</option>
        </select>
      </label>
    </section>
  );
}
