# Deploy

This directory contains the alpha CRDs, RBAC, operator and read-only API
Deployments, and sample resources. Helm is not part of the current distribution.

The current API group is `kember.openflood.org`. Older `kember.dev` resources are
not converted automatically; treat a group change as an alpha migration and
remove the old CRDs only after their resources have been backed up or retired.

The operator image must already be available to the target cluster. For a kind
cluster, load it before installation:

```bash
kind load docker-image --name kember-e2e kember-operator:e2e
KEMBER_OPERATOR_IMAGE=kember-operator:e2e ./deploy/install.sh
```

`install.sh` applies the namespace, CRDs, RBAC, and Deployment, then waits for
the operator rollout. It uses the current kubectl context. The API is packaged
separately because its JVM image must also be built and made available to the
cluster.

## Read-only API

Build `//apps/kember-api:kember-api`, package the generated
`kember-api_deploy.jar` with `deploy/api/Dockerfile`, and load the image into the
cluster. Then apply the namespace-scoped reader and workload:

```bash
kubectl apply -f deploy/rbac/kember-api.yaml
kubectl apply -f deploy/api/api.yaml
kubectl -n kember-system rollout status deployment/kember-api
kubectl -n kember-system port-forward service/kember-api 18081:8080
curl http://127.0.0.1:18081/api/v1/namespaces
```

The checked-in manifest reads only `kember-system`. Selecting another namespace
requires moving the Role and RoleBinding and changing `KEMBER_NAMESPACE` to the
same value. Do not replace the RoleBinding with a ClusterRoleBinding merely to
select several namespaces; bind a common ClusterRole separately in each allowed
namespace when that mode is implemented.

After installation, apply a sample WorkerPool and TaskRun from
`deploy/samples/`. `kember-platform-admin` is intended to manage WorkerPools;
`kember-taskrun-developer` is intended to create TaskRuns and patch
`spec.cancel`.

## Observe a TaskRun

```bash
kubectl -n kember-e2e get taskrun echo -o wide
kubectl -n kember-e2e describe taskrun echo
```

The phase is monotonic: `Pending` or `Running` transitions to one terminal
phase (`Succeeded`, `Failed`, `TimedOut`, `Rejected`, or `Cancelled`). A caller
can request cancellation before completion:

```bash
kubectl -n <namespace> patch taskrun <name> --type=merge \
  -p '{"spec":{"cancel":true}}'
```

## Observe metrics

The Deployment exposes metrics on port 8080. For local inspection:

```bash
kubectl -n kember-system port-forward deployment/kember-operator 18080:8080
curl http://127.0.0.1:18080/metrics
```

The alpha lifecycle families are `kember_workerpool_ready_workers`,
`kember_workerpool_leased_workers`, `kember_taskrun_active_duration_seconds`,
`kember_taskrun_total`, `kember_worker_termination_requests_total`, and
`kember_taskrun_assignment_wait_seconds`. Metrics are diagnostic signals and
do not replace TaskRun status.
