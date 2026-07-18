# Kember

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
- Kotlin API and TypeScript UI are repository bootstraps
- No compatibility, Helm, or production-scale guarantees yet

The current API group is `kember.dev/v1alpha1` and may change before the first
public release.

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
apiVersion: kember.dev/v1alpha1
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

## Contributing

Issues and focused pull requests are welcome. Please include a reproducible
test or scenario for lifecycle behavior changes.

## License

License terms have not been finalized yet.
