import { useCallback } from "react";
import { Link, useParams } from "react-router-dom";
import { useApi } from "../api/ApiContext";
import { usePollingResource } from "../api/usePollingResource";
import { Conditions } from "../components/Conditions";
import { InitialError, Loading, RefreshError } from "../components/States";
import { StatusBadge } from "../components/StatusBadge";
import { LastUpdated } from "../components/LastUpdated";
import { Timeline } from "../components/Timeline";
import { formatActiveDuration, formatDuration } from "../lib/format";

export function TaskRunDetailPage({ namespace }: { namespace: string }) {
  const api = useApi();
  const name = useParams().name ?? "";
  const loader = useCallback((signal: AbortSignal) => api.getTaskRun(namespace, name, signal), [api, namespace, name]);
  const resource = usePollingResource(loader);
  if (resource.loading) return <Loading />;
  if (!resource.data) return <InitialError error={resource.error} retry={resource.retry} />;
  const taskRun = resource.data;
  return (
    <section>
      <p className="eyebrow">Task run</p><div className="page-heading"><h1>{taskRun.name}</h1><LastUpdated value={resource.updatedAt} refreshing={resource.refreshing} /></div>
      {resource.error !== null && <RefreshError />}
      <div className="detail-status"><StatusBadge phase={taskRun.phase} /></div>
      <dl className="facts">
        <div><dt>Worker pool</dt><dd><Link to={`/worker-pools/${encodeURIComponent(taskRun.workerPool)}`}>{taskRun.workerPool}</Link></dd></div>
        <div><dt>Worker</dt><dd>{taskRun.assignedWorker ?? "—"}</dd></div>
        <div><dt>Queue wait</dt><dd>{formatDuration(taskRun.queueWaitSeconds)}</dd></div>
        <div><dt>Active</dt><dd>{formatActiveDuration(taskRun)}</dd></div>
      </dl>
      <h2>Lifecycle</h2><Timeline createdAt={taskRun.createdAt} dispatchedAt={taskRun.dispatchedAt} completedAt={taskRun.completedAt} />
      <h2>Conditions</h2><Conditions conditions={taskRun.conditions} />
    </section>
  );
}
