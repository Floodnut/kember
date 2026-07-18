import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useApi } from "./api/ApiContext";
import { AppShell } from "./components/AppShell";
import { Empty, InitialError, Loading } from "./components/States";
import { TaskRunDetailPage } from "./pages/TaskRunDetailPage";
import { TaskRunListPage } from "./pages/TaskRunListPage";
import { WorkerPoolDetailPage } from "./pages/WorkerPoolDetailPage";
import { WorkerPoolListPage } from "./pages/WorkerPoolListPage";

interface Scope {
  cluster: string;
  namespace: string;
}

export function App() {
  const api = useApi();
  const [scope, setScope] = useState<Scope | null>(null);
  const [error, setError] = useState<unknown | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setError(null);
    api.getNamespaces(controller.signal).then((response) => {
      const first = response.items[0];
      if (first) setScope({ cluster: first.cluster, namespace: first.name });
    }).catch((cause: unknown) => {
      if (!(cause instanceof DOMException && cause.name === "AbortError")) setError(cause);
    });
    return () => controller.abort();
  }, [api, attempt]);

  return (
    <AppShell cluster={scope?.cluster ?? "local"} namespace={scope?.namespace ?? "—"}>
      {!scope && !error && <Loading />}
      {!scope && error !== null && <InitialError error={error} retry={() => setAttempt((value) => value + 1)} />}
      {scope && (
        <Routes>
          <Route path="/" element={<Navigate to="/worker-pools" replace />} />
          <Route path="/worker-pools" element={<WorkerPoolListPage namespace={scope.namespace} />} />
          <Route path="/worker-pools/:name" element={<WorkerPoolDetailPage namespace={scope.namespace} />} />
          <Route path="/task-runs" element={<TaskRunListPage namespace={scope.namespace} />} />
          <Route path="/task-runs/:name" element={<TaskRunDetailPage namespace={scope.namespace} />} />
          <Route path="*" element={<Empty>Page not found</Empty>} />
        </Routes>
      )}
    </AppShell>
  );
}
