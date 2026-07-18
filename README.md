# Kember

Project site: [openflood.org/kember](https://openflood.org/kember)

Kubernetes-native worker lifecycle control for short-lived container workloads.

Kember keeps a declared pool of warm, single-use worker Pods, assigns one Pod to
each `TaskRun`, executes the image command, and replaces the Pod after terminal
completion. It also supports a conventional Job-per-task execution path.

Kember is a small control-plane layer. It does not replace the Kubernetes
scheduler, Jobs, or autoscalers; it makes worker preparation, assignment,
timeouts, terminal state, and capacity observable and declarative.

## Status

Kember is an early alpha and its API is not stable yet.

- Go operator with `WorkerPool` and `TaskRun` CRDs
- Job and WarmLease execution paths
- RBAC, lifecycle metrics, unit tests, and kind-based E2E scenarios
- Namespace-scoped, read-only Kotlin API; TypeScript UI remains a bootstrap
- No compatibility, Helm, or production-scale guarantees yet

The current API group is `kember.openflood.org/v1alpha1` and may change before the first
public release.

This alpha uses `kember.openflood.org` as its API group. There is no conversion
webhook from the earlier `kember.dev` experiment; back up and remove old CRDs
only when no old resources remain, then apply the current manifests.

## Alpha support matrix

| Execution mode | Lifecycle profile | Capacity | Status |
|---|---|---|---|
| `job` | `runToCompletion` | Kubernetes Job per TaskRun | Supported |
| `exec` | `warmLease` | Fixed pool, one TaskRun per worker | Supported |
| Any other mode/profile | — | — | Not supported |

The alpha API guarantees one terminal TaskRun phase and an immutable resolved
execution template after dispatch. It does not guarantee API compatibility,
multi-cluster operation, Helm packaging, reusable workers, or production-scale
performance.

## Repository layout

```text
apps/kember-operator  Go Kubernetes operator
apps/kember-api       Kotlin control API bootstrap
apps/kember-ui        TypeScript UI bootstrap
packages              Shared contracts
deploy                CRDs, RBAC, operator, and samples
tests                 E2E and benchmark harnesses
tools                 Bazel toolchain configuration
```

## Requirements

- Go 1.25+
- Java 17+
- Bazel 9.1.0
- Docker, kind, and kubectl

## Build and test

```bash
go test ./...
bazel test //...
```

## Install on a cluster

The alpha distribution uses plain Kubernetes manifests. Build an operator
image, make it available to the target cluster, and install the control plane:

```bash
go build -o /tmp/kember-operator ./apps/kember-operator
docker build -f deploy/operator/Dockerfile -t kember-operator:e2e /tmp
kind load docker-image --name kember-e2e kember-operator:e2e
KEMBER_OPERATOR_IMAGE=kember-operator:e2e ./deploy/install.sh
```

The installer applies the namespace, CRDs, RBAC, and operator Deployment to the
current kubectl context. It does not install a Helm chart or create a tenant
namespace.

For a quick lifecycle check after installation:

```bash
kubectl apply -f deploy/samples/e2e-success.yaml
kubectl -n kember-e2e get taskrun echo -o wide
kubectl -n kember-e2e describe taskrun echo
```

To request cancellation for a non-terminal TaskRun:

```bash
kubectl -n <namespace> patch taskrun <name> --type=merge \
  -p '{"spec":{"cancel":true}}'
```

The operator exposes lifecycle metrics on port 8080:

```bash
kubectl -n kember-system port-forward deployment/kember-operator 18080:8080
curl http://127.0.0.1:18080/metrics
```

The alpha metric families are `kember_workerpool_ready_workers`,
`kember_workerpool_leased_workers`, `kember_taskrun_active_duration_seconds`,
`kember_taskrun_total`, `kember_worker_termination_requests_total`, and
`kember_taskrun_assignment_wait_seconds`.

## Read-only API

The Kotlin API exposes the current `WorkerPool` and `TaskRun` projections for
one configured namespace. Resource identity always includes `cluster`,
`namespace`, and `name`; the alpha cluster identifier is `local`.

```text
GET /healthz
GET /api/v1/namespaces
GET /api/v1/namespaces/{namespace}/worker-pools
GET /api/v1/namespaces/{namespace}/worker-pools/{name}
GET /api/v1/namespaces/{namespace}/task-runs
GET /api/v1/namespaces/{namespace}/task-runs/{name}
```

Build the API with Bazel:

```bash
bazel build //apps/kember-api:kember-api
bazel test //apps/kember-api:all
```

The API process also serves the read-only dashboard from the same origin. Build
`//apps/kember-ui:build`, set `KEMBER_UI_DIR` to that `dist` directory, and open
the port-forwarded Service:

```bash
kubectl -n kember-system port-forward service/kember-api 18081:8080
open http://127.0.0.1:18081/
```

`KEMBER_NAMESPACE` and `KEMBER_UI_DIR` are required, and `KEMBER_API_PORT`
defaults to `8080`. The
plain manifests in `deploy/api` use `kember-system` and the API ServiceAccount
has only `get` and `list` on Kember resources in that namespace. To select
another namespace, change both the environment value and the namespaced
Role/RoleBinding placement. Multi-namespace and multi-cluster reads are not
implemented in the alpha.

Run the kind-based lifecycle scenarios:

```bash
kind create cluster --name kember-e2e
tests/e2e/warm-single-use.sh
tests/e2e/warm-concurrent-failures.sh
tests/e2e/warm-status-clean-cluster.sh
```

The benchmark harness includes one-shot Checkov and Trivy smoke commands:

```bash
WARMUP_ITERATIONS=0 ITERATIONS=1 tests/benchmark/checkov-warmlease.sh
WARMUP_ITERATIONS=0 ITERATIONS=1 tests/benchmark/trivy-warmlease.sh
```

## Example

```yaml
apiVersion: kember.openflood.org/v1alpha1
kind: TaskRun
metadata:
  name: scan-source
spec:
  workerPoolRef:
    name: scanner-warm
  input:
    ref: s3://security-artifacts/project-a/source.tar.gz
  timeoutSeconds: 60
```

See [`deploy/samples`](deploy/samples) for the corresponding `WorkerPool` and
additional manifests.

`WorkerPool` is platform-owned and defines the image, command, security
context, input prefix policy, timeout policy, and worker capacity. `TaskRun` is
namespaced and requests one execution without overriding that template. A
TaskRun moves monotonically through `Pending` and `Running` to one terminal
phase: `Succeeded`, `Failed`, `TimedOut`, `Rejected`, or `Cancelled`.

## Contributing

Issues and focused pull requests are welcome. Please include a reproducible
test or scenario for lifecycle behavior changes.

## License

License terms have not been finalized yet.
