import { useCallback } from "react";
import { Link } from "react-router-dom";
import { useApi } from "../api/ApiContext";
import { usePollingResource } from "../api/usePollingResource";
import { Empty, InitialError, Loading, RefreshError } from "../components/States";
import { ResourceTable } from "../components/ResourceTable";
import { LastUpdated } from "../components/LastUpdated";

export function WorkerPoolListPage({ namespace }: { namespace: string }) {
  const api = useApi();
  const loader = useCallback((signal: AbortSignal) => api.listWorkerPools(namespace, signal), [api, namespace]);
  const resource = usePollingResource(loader);
  if (resource.loading) return <Loading />;
  if (!resource.data) return <InitialError error={resource.error} retry={resource.retry} />;
  return (
    <section>
      <div className="page-heading"><div><p className="eyebrow">Worker capacity</p><h1>Worker pools</h1></div><LastUpdated value={resource.updatedAt} refreshing={resource.refreshing} /></div>
      {resource.error !== null && <RefreshError />}
      {resource.data.items.length === 0 ? <Empty>No worker pools</Empty> : (
        <ResourceTable label="Worker pools" headers={["Name", "Mode", "Profile", "Desired", "Starting", "Ready", "Leased", "Terminating"]}>
          {resource.data.items.map((pool) => <tr key={pool.name}>
            <td><Link to={`/worker-pools/${encodeURIComponent(pool.name)}`}>{pool.name}</Link></td>
            <td>{pool.executionMode ?? "—"}</td><td>{pool.lifecycleProfile ?? "—"}</td>
            <td>{pool.capacity.desired}</td><td>{pool.capacity.starting}</td><td>{pool.capacity.ready}</td>
            <td>{pool.capacity.leased}</td><td>{pool.capacity.terminating}</td>
          </tr>)}
        </ResourceTable>
      )}
    </section>
  );
}
