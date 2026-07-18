# Kember

Kember는 Kubernetes에서 미리 준비한 Pod를 task에 독점 배정하고, 한 번 실행한 worker를 폐기·보충하는 warm single-use worker pool Operator입니다.

현재 alpha 구현은 두 execution path를 지원합니다.

- `job`: `TaskRun`마다 Kubernetes Job을 생성한다.
- `exec` + `warmLease`: Ready worker Pod를 Lease로 배정하고 command를 실행한 뒤 해당 Pod를 교체한다.

Kember는 Kubernetes Job, scheduler 또는 autoscaler를 대체하지 않습니다. Worker template, 입력 허용 범위, timeout, terminal state와 warm capacity lifecycle을 Kubernetes CRD로 선언하고 조정하는 좁은 control plane입니다.

## 현재 상태

- 구현됨: Go Operator, `WorkerPool`/`TaskRun` CRD, RBAC, Job lifecycle, WarmLease single-use lifecycle, E2E와 benchmark harness
- 골격만 존재: Kotlin control API와 TypeScript UI
- 아직 없음: 안정 API, Helm chart, multi-node 검증, reusable application process, plugin adapter

API는 `kember.dev/v1alpha1` alpha group을 사용합니다. `kember.dev` 소유권이 확정되지 않으면 public release 전에 소유한 domain 기반 group으로 변경합니다.

## Repository

```text
apps/kember-operator  Go Kubernetes Operator
apps/kember-api       Kotlin control API bootstrap
apps/kember-ui        TypeScript UI bootstrap
packages              공유 contract
deploy                CRD, RBAC, Operator와 sample manifest
tests                 E2E와 benchmark harness
tools                 Bazel toolchain 설정
```

## 요구 환경

- Go 1.25 이상
- Bazel 9.1.0
- Java 17 이상
- Docker, kind, kubectl

## Build and test

```bash
go test ./...
bazel test //...
```

WarmLease E2E는 `kember-e2e` kind cluster를 사용합니다.

```bash
kind create cluster --name kember-e2e
tests/e2e/warm-single-use.sh
tests/e2e/warm-concurrent-failures.sh
tests/e2e/warm-status-clean-cluster.sh
```

E2E harness는 Operator와 fixture image를 빌드해 cluster에 적재하고, `kember.dev` CRD와 `kember-system` namespace를 적용합니다.

## Kubernetes API

`WorkerPool`은 platform owner가 관리하는 실행·보안·capacity template이고, `TaskRun`은 해당 template으로 한 번 실행할 것을 요청하는 namespaced resource입니다.

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

전체 예시는 `deploy/samples`에서 확인할 수 있습니다.
