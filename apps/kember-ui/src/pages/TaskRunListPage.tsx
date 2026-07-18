import { useCallback } from "react";
import { Link } from "react-router-dom";
import { useApi } from "../api/ApiContext";
import { usePollingResource } from "../api/usePollingResource";
import { ResourceTable } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { LastUpdated } from "../components/LastUpdated";
import { Empty, InitialError, Loading, RefreshError } from "../components/States";
import { formatActiveDuration, formatDuration } from "../lib/format";

export function TaskRunListPage({ namespace }: { namespace: string }) {
  const api = useApi();
  const loader = useCallback((signal: AbortSignal) => api.listTaskRuns(namespace, signal), [api, namespace]);
  const resource = usePollingResource(loader);
  if (resource.loading) return <Loading />;
  if (!resource.data) return <InitialError error={resource.error} retry={resource.retry} />;
  return (
    <section>
      <div className="page-heading"><div><p className="eyebrow">Worker lifecycle</p><h1>Task runs</h1></div><LastUpdated value={resource.updatedAt} refreshing={resource.refreshing} /></div>
      {resource.error !== null && <RefreshError />}
      {resource.data.items.length === 0 ? <Empty>No task runs</Empty> : (
        <ResourceTable label="Task runs" headers={["Name", "Phase", "Worker pool", "Worker", "Queue", "Active"]}>
          {resource.data.items.map((taskRun) => <tr key={taskRun.name}>
            <td><Link to={`/task-runs/${encodeURIComponent(taskRun.name)}`}>{taskRun.name}</Link></td>
            <td><StatusBadge phase={taskRun.phase} /></td><td>{taskRun.workerPool}</td>
            <td>{taskRun.assignedWorker ?? "—"}</td><td>{formatDuration(taskRun.queueWaitSeconds)}</td>
            <td>{formatActiveDuration(taskRun)}</td>
          </tr>)}
        </ResourceTable>
      )}
    </section>
  );
}
