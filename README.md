# Kember

Project site: [openflood.org/kember](https://openflood.org/kember)

Kubernetes-native worker lifecycle control for short-lived container workloads.

Kember keeps a declared pool of warm, single-use worker Pods, assigns one Pod to
each `TaskRun`, executes the image command, and replaces the Pod after terminal
completion. It also supports a conventional Job-per-task execution path.

Kember is a small control-plane layer. It does not replace the Kubernetes
scheduler, Jobs, or autoscalers; it makes worker preparation, assignment,
timeouts, terminal state, and capacity observable and declarative.

Kember is best suited for command-style workers where startup, dependency
preparation, or per-task lifecycle governance matters. Examples include
security scanners, validators, analyzers, and other short-lived tools that can
accept an input reference and return a terminal result.

Kember is not a workflow engine, serverless serving framework, general
autoscaler, queue broker, or sandbox for arbitrary untrusted images.

## Status

Kember is an early alpha and its API is not stable yet.

- Go operator with `WorkerPool` and `TaskRun` CRDs
- Job and WarmLease execution paths
- RBAC, lifecycle metrics, unit tests, and kind-based E2E scenarios
- Namespace-scoped, read-only Kotlin API and TypeScript dashboard
- No compatibility, Helm, or production-scale guarantees yet

The current API group is `kember.openflood.org/v1alpha1`. There is no
conversion webhook from older experimental API groups; treat group changes as
alpha migrations.

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

Kember alpha targets Kubernetes 1.34-compatible clusters. The operator is built
against the Kubernetes v0.34.x Go client family and controller-runtime v0.22.x.

Local builds use Bazel toolchains rather than whatever happens to be installed
on the host:

| Area | Version |
|---|---|
| Bazel | 9.1.0 |
| Go | 1.25.0 |
| JVM | Java 17 |
| Kotlin language/API | 2.2 |
| Kotlin compiler distribution | provided by `rules_kotlin` 2.4.0 |
| Node.js for the dashboard | 22.12.0 |

To try Kember on a cluster, you need a current `kubectl` context and a container
image registry or build pipeline that the cluster can pull from.

## Quickstart

```bash
export KEMBER_IMAGE=registry.example.com/kember/kember-operator:e2e

go build -o /tmp/kember-operator ./apps/kember-operator
docker build -f deploy/operator/Dockerfile -t kember-operator:e2e /tmp

docker tag kember-operator:e2e "${KEMBER_IMAGE}"
docker push "${KEMBER_IMAGE}"

KEMBER_OPERATOR_IMAGE="${KEMBER_IMAGE}" ./deploy/install.sh
kubectl apply -f deploy/samples/e2e-success.yaml
kubectl -n kember-e2e get taskrun echo -o wide
kubectl -n kember-e2e describe taskrun echo
```

The sample creates a `WorkerPool` and a `TaskRun`. A successful run reaches one
terminal phase, usually `Succeeded`.

For local-only smoke tests, kind also works if you load the image into the kind
cluster instead of pushing it to a registry.

For a WarmLease check:

```bash
kubectl apply -f deploy/samples/e2e-warm-single-use.yaml
kubectl -n kember-warm-e2e get workerpool echo-warm -o wide
kubectl -n kember-warm-e2e get taskrun echo-warm -o wide
```

Useful troubleshooting commands:

```bash
kubectl -n kember-system get pods
kubectl -n kember-system logs deployment/kember-operator
kubectl get crd | grep kember.openflood.org
kubectl -n kember-e2e get workerpool,taskrun
kubectl -n kember-e2e describe taskrun echo
```

## Build and test

```bash
go test ./...
bazel test //...
```

## Documentation

- [Alpha API](docs/api.md)
- [v0.1 Alpha API Contract](docs/v0.1-api-contract.md)
- [Deployment manifests](deploy/README.md)

## Install on a cluster

The alpha distribution uses plain Kubernetes manifests. Build an operator
image, make it available to the target cluster, and install the control plane:

```bash
export KEMBER_IMAGE=registry.example.com/kember/kember-operator:e2e

go build -o /tmp/kember-operator ./apps/kember-operator
docker build -f deploy/operator/Dockerfile -t kember-operator:e2e /tmp
docker tag kember-operator:e2e "${KEMBER_IMAGE}"
docker push "${KEMBER_IMAGE}"
KEMBER_OPERATOR_IMAGE="${KEMBER_IMAGE}" ./deploy/install.sh
```

The installer applies the namespace, CRDs, RBAC, and operator Deployment to the
current kubectl context. It does not install a Helm chart, install the read-only
API, or create tenant namespaces beyond those declared by the sample manifests.

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

## Read-only API and dashboard

The Kotlin API exposes current `WorkerPool` and `TaskRun` projections for one
configured namespace. It also serves the read-only dashboard from the same
origin.

```text
GET /healthz
GET /api/v1/namespaces
GET /api/v1/namespaces/{namespace}/worker-pools
GET /api/v1/namespaces/{namespace}/worker-pools/{name}
GET /api/v1/namespaces/{namespace}/task-runs
GET /api/v1/namespaces/{namespace}/task-runs/{name}
```

Build the API and dashboard with Bazel:

```bash
bazel build //apps/kember-api:kember-api_deploy.jar //apps/kember-ui:build
bazel test //apps/kember-api:all
```

The plain manifests in `deploy/api` configure `KEMBER_NAMESPACE=kember-system`
and serve the bundled UI from `/app/ui`. Open the port-forwarded Service:

```bash
kubectl -n kember-system port-forward service/kember-api 18081:8080
open http://127.0.0.1:18081/
```

The API ServiceAccount has only `get` and `list` on Kember resources in the
configured namespace. To select another namespace, change both
`KEMBER_NAMESPACE` and the namespaced Role/RoleBinding placement. Multi-namespace
and multi-cluster reads are not implemented in the alpha. See
[docs/api.md](docs/api.md) for the current alpha API surface.

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

Apache License 2.0. See [LICENSE](LICENSE).
