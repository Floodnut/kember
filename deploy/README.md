# Deploy

This directory contains the alpha CRDs, RBAC, operator Deployment, and sample
resources. Helm is not part of the current distribution.

The operator image must already be available to the target cluster. For a kind
cluster, load it before installation:

```bash
kind load docker-image --name kember-e2e kember-operator:e2e
KEMBER_OPERATOR_IMAGE=kember-operator:e2e ./deploy/install.sh
```

`install.sh` applies the namespace, CRDs, RBAC, and Deployment, then waits for
the operator rollout. It uses the current kubectl context.

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
