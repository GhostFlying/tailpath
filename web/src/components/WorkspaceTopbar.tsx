import { Waypoints } from "lucide-react";
import type { ReactNode } from "react";
import { NavLink } from "react-router-dom";

interface Props {
  connection?: string;
  metrics?: ReactNode;
  className?: string;
}

export function WorkspaceTopbar({
  connection = "live",
  metrics,
  className = "",
}: Props) {
  return (
    <header className={`topbar ${className}`.trim()}>
      <NavLink className="brand" to="/" aria-label="Tailpath Live">
        <Waypoints size={22} />
        <strong>Tailpath</strong>
      </NavLink>
      <nav className="workspace-tabs" aria-label="Workspace">
        <NavLink to="/" end>
          Live
        </NavLink>
        <NavLink to="/history">History</NavLink>
      </nav>
      {metrics ? <div className="headline-metrics">{metrics}</div> : null}
      <div className={`live-state ${connection}`}>
        <span />
        {connection}
      </div>
    </header>
  );
}
