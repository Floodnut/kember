import { useCallback } from "react";
import { Link, useParams } from "react-router-dom";
import { useApi } from "../api/ApiContext";
import { usePollingResource } from "../api/usePollingResource";
import { Conditions } from "../components/Conditions";
import { InitialError, Loading, RefreshError } from "../components/States";
import { LastUpdated } from "../components/LastUpdated";

export function WorkerPoolDetailPage({ namespace }: { namespace: string }) {
  const api = useApi();
  const name = useParams().name ?? "";
  const poolLoader = useCallback((signal: AbortSignal) => api.getWorkerPool(namespace, name, signal), [api, namespace, name]);
  const tasksLoader = useCallback((signal: AbortSignal) => api.listTaskRuns(namespace, signal), [api, namespace]);
  const pool = usePollingResource(poolLoader);
  const tasks = usePollingResource(tasksLoader);
  if (pool.loading || tasks.loading) return <Loading />;
  if (!pool.data) return <InitialError error={pool.error} retry={pool.retry} />;
  if (!tasks.data) return <InitialError error={tasks.error} retry={tasks.retry} />;
  const related = tasks.data.items.filter((taskRun) => taskRun.workerPool === pool.data?.name);
  return (
    <section>
      <p className="eyebrow">Worker pool</p><div className="page-heading"><h1>{pool.data.name}</h1><LastUpdated value={pool.updatedAt} refreshing={pool.refreshing || tasks.refreshing} /></div>
      {(pool.error !== null || tasks.error !== null) && <RefreshError />}
      <dl className="facts"><div><dt>Mode</dt><dd>{pool.data.executionMode ?? "—"}</dd></div><div><dt>Profile</dt><dd>{pool.data.lifecycleProfile ?? "—"}</dd></div></dl>
      <div className="capacity-grid">{Object.entries(pool.data.capacity).map(([label, value]) => <div key={label}><span>{label}</span><strong>{value}</strong></div>)}</div>
      <h2>Conditions</h2><Conditions conditions={pool.data.conditions} />
      <h2>Task runs</h2>
      {related.length === 0 ? <p className="muted">No related task runs</p> : <ul className="resource-links">{related.map((taskRun) => <li key={taskRun.name}><Link to={`/task-runs/${encodeURIComponent(taskRun.name)}`}>{taskRun.name}</Link></li>)}</ul>}
    </section>
  );
}
