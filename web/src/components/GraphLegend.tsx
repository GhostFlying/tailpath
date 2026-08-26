import {
  Activity,
  CircleHelp,
  Link2,
  Server,
  TriangleAlert,
} from "lucide-react";

export function GraphLegend() {
  return (
    <div className="graph-legend" aria-label="Topology legend">
      <div className="legend-section">
        <strong>Path</strong>
        <div className="legend-group">
          <span>
            <i className="path-glyph direct" /> Direct
          </span>
          <span>
            <i className="path-glyph derp" /> DERP
          </span>
          <span>
            <i className="path-glyph peer_relay" /> Peer Relay
          </span>
          <span>
            <i className="path-glyph unknown" /> Unknown
          </span>
        </div>
      </div>
      <div className="legend-section">
        <strong>Activity</strong>
        <div className="legend-group">
          <span>
            <i className="legend-activity active" /> Active
          </span>
          <span>
            <i className="legend-activity recent" /> Recent
          </span>
        </div>
      </div>
      <div className="legend-section">
        <strong>Node telemetry</strong>
        <div className="legend-group node-legend">
          <span>
            <i className="legend-device">
              <Server size={13} />
            </i>{" "}
            Platform device
          </span>
          <span>
            <i className="legend-runtime-badge">
              <Activity size={9} />
            </i>{" "}
            Runtime telemetry
          </span>
          <span>
            <i className="legend-node peer-only" /> Peer only
          </span>
          <span>
            <i className="legend-clock">
              <TriangleAlert size={13} />
            </i>{" "}
            Clock skew
          </span>
        </div>
      </div>
      <div className="legend-section">
        <strong>Relay identity</strong>
        <div className="legend-group identity-legend">
          <span>
            <i className="legend-identity partial">
              <Link2 size={12} />
            </i>{" "}
            Partial
          </span>
          <span>
            <i className="legend-identity anonymous">
              <CircleHelp size={12} />
            </i>{" "}
            Anonymous
          </span>
          <span>
            <i className="legend-identity conflict">
              <TriangleAlert size={12} />
            </i>{" "}
            Conflict
          </span>
        </div>
      </div>
    </div>
  );
}
