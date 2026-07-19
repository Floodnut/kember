# Kember Alpha API

Kember exposes two Kubernetes custom resources in
`kember.openflood.org/v1alpha1`:

- `WorkerPool`: a platform-owned execution template and worker capacity policy.
- `TaskRun`: a namespaced request to execute one unit of work through a
  `WorkerPool`.

The API is alpha. Field names, defaults, and lifecycle semantics can change
before a stable release.

## WorkerPool

`WorkerPool` declares how work is executed. A tenant `TaskRun` cannot override
the image, command, service account, resource limits, security context, or input
policy owned by the pool.

```yaml
apiVersion: kember.openflood.org/v1alpha1
kind: WorkerPool
metadata:
  name: echo-warm
  namespace: kember-warm-e2e
spec:
  execution:
    mode: exec
    commandTemplate:
      - /bin/sh
      - -c
      - 'sleep 3; test "$1" = "s3://security-artifacts/project-a/source.tar.gz"'
      - kember-task
      - "{{input.ref}}"
  lifecycle:
    profile: warmLease
    maxTasksPerWorker: 1
  capacity:
    policy: fixed
    size: 2
  template:
    image: busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
    command: ["/bin/sh", "-c"]
    args: ["touch /tmp/kember-ready; exec sleep 3600"]
    serviceAccountName: warm-worker
    inputPolicy:
      allowedPrefixes: ["s3://security-artifacts/project-a/"]
    resources:
      requests:
        cpu: "10m"
        memory: "16Mi"
      limits:
        cpu: "50m"
        memory: "32Mi"
    readinessProbe:
      exec:
        command: ["/bin/sh", "-c", "test -f /tmp/kember-ready"]
      periodSeconds: 1
  taskPolicy:
    queueTimeoutSeconds: 30
    timeoutSeconds: 60
    retentionSeconds: 60
```

### Supported modes

| Execution mode | Lifecycle profile | Capacity model | Status |
|---|---|---|---|
| `job` | `runToCompletion` | One Kubernetes Job per TaskRun | Supported |
| `exec` | `warmLease` | Fixed warm Pod pool, one TaskRun per worker | Supported |

Other execution modes, lifecycle profiles, autoscaling policies, reusable
workers, and sidecar worker contracts are not part of the alpha API.

### WorkerPool status

Warm pools report capacity in `status.capacity`:

- `desired`: requested fixed pool size.
- `starting`: non-ready worker Pods in the current pool generation.
- `ready`: ready and unleased worker Pods.
- `leased`: worker Pods assigned to a TaskRun.
- `terminating`: old or used worker Pods being drained.

`status.conditions` reports `Ready`, `Progressing`, and `Degraded`.

## TaskRun

`TaskRun` requests one execution. It references a WorkerPool in the same
namespace and supplies only parameters and an input reference.

```yaml
apiVersion: kember.openflood.org/v1alpha1
kind: TaskRun
metadata:
  name: echo-warm
  namespace: kember-warm-e2e
spec:
  workerPoolRef:
    name: echo-warm
  input:
    ref: s3://security-artifacts/project-a/source.tar.gz
  timeoutSeconds: 30
```

`input.ref` must match one of the WorkerPool
`spec.template.inputPolicy.allowedPrefixes` values. Kember validates the
reference but does not download input data or inject storage credentials; the
worker image owns its own data-plane behavior.

### TaskRun status

`TaskRun.status.phase` moves monotonically to one terminal phase:

- `Succeeded`
- `Failed`
- `TimedOut`
- `Rejected`
- `Cancelled`

The status can include:

- `resolvedTemplate`: the image, command, resource, deadline, and lifecycle
  snapshot selected for the run.
- `jobRef`: the owned Job for `job` execution.
- `workerRef`: the assigned warm worker for `warmLease` execution.
- `dispatchedAt` and `completedAt`: lifecycle timestamps.
- `conditions`: terminal reason and message.

Terminal TaskRuns are not retried or re-opened.

## Read-only HTTP API

The Kotlin API serves namespace-scoped read-only projections for the dashboard:

```text
GET /healthz
GET /api/v1/namespaces
GET /api/v1/namespaces/{namespace}/worker-pools
GET /api/v1/namespaces/{namespace}/worker-pools/{name}
GET /api/v1/namespaces/{namespace}/task-runs
GET /api/v1/namespaces/{namespace}/task-runs/{name}
```

The alpha API is configured for one namespace through `KEMBER_NAMESPACE`.
Multi-namespace and multi-cluster reads are not implemented yet.

## Security boundary

Kember constrains how a TaskRun selects an execution template, but it is not a
sandbox for untrusted images. Treat WorkerPool images as trusted platform
artifacts and use Kubernetes controls such as service accounts, RBAC, resource
limits, non-root containers, and read-only filesystems.

The alpha does not provide OIDC login, OPA policy adapters, Keycloak
integration, admission webhooks, or plugin bindings.
