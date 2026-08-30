import { Waypoints } from "lucide-react";
import type { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import { useCapabilities } from "../hooks/useCapabilities";

interface Props {
  connection: WorkspaceConnection;
  metrics?: ReactNode;
  className?: string;
}

export interface WorkspaceConnection {
  state: "connecting" | "live" | "reconnecting" | "reachable" | "error";
  label: string;
  ariaLabel: string;
}

export function WorkspaceTopbar({
  connection,
  metrics,
  className = "",
}: Props) {
  const { deviceDirectoryEnabled } = useCapabilities();
  return (
    <header
      className={`topbar ${deviceDirectoryEnabled ? "has-devices" : ""} ${className}`.trim()}
    >
      <NavLink className="brand" to="/" aria-label="Tailpath Live">
        <Waypoints size={22} />
        <strong>Tailpath</strong>
      </NavLink>
      <nav className="workspace-tabs" aria-label="Workspace">
        <NavLink to="/" end>
          Live
        </NavLink>
        <NavLink to="/history">History</NavLink>
        {deviceDirectoryEnabled ? (
          <NavLink to="/devices">Devices</NavLink>
        ) : null}
      </nav>
      {metrics ? <div className="headline-metrics">{metrics}</div> : null}
      <div
        className={`live-state ${connection.state}`}
        aria-label={connection.ariaLabel}
      >
        <span />
        {connection.label}
      </div>
    </header>
  );
}
