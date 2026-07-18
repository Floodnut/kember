import { NavLink, Navigate, Route, Routes } from "react-router-dom";

export function App() {
  return (
    <div>
      <header>
        <strong>Kember</strong>
        <nav aria-label="Primary navigation">
          <NavLink to="/worker-pools">Worker pools</NavLink>
          <NavLink to="/task-runs">Task runs</NavLink>
        </nav>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<Navigate to="/worker-pools" replace />} />
          <Route path="/worker-pools" element={<h1>Worker pools</h1>} />
          <Route path="/worker-pools/:name" element={<h1>Worker pool</h1>} />
          <Route path="/task-runs" element={<h1>Task runs</h1>} />
          <Route path="/task-runs/:name" element={<h1>Task run</h1>} />
        </Routes>
      </main>
    </div>
  );
}
