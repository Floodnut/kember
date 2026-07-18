import type { ReactNode } from "react";
import { NavLink } from "react-router-dom";

export function AppShell({ cluster, namespace, children }: {
  cluster: string;
  namespace: string;
  children: ReactNode;
}) {
  return (
    <div className="app-shell">
      <header className="topbar">
        <NavLink className="brand" to="/worker-pools">Kember</NavLink>
        <nav aria-label="Primary navigation">
          <NavLink to="/worker-pools">Worker pools</NavLink>
          <NavLink to="/task-runs">Task runs</NavLink>
        </nav>
        <span className="scope">{cluster} / {namespace}</span>
      </header>
      <main className="content">{children}</main>
    </div>
  );
}
