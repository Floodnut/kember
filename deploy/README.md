# Deploy

Kember CRD, RBAC, Helm chart, sample manifest를 이 디렉터리에 둔다.

초기 순서는 `operator/namespace.yaml`, `crd/`, `rbac/`, `operator/operator.yaml`, `samples/`다.

v0.1 설치 순서는 다음과 같다.

1. `operator/namespace.yaml`로 `kember-system` namespace를 만든다.
2. `crd/`의 `WorkerPool`, `TaskRun` CRD를 적용한다.
3. `rbac/kember-operator.yaml`로 Operator service account와 controller 권한을 적용한다.
4. `operator/operator.yaml`로 Operator Deployment를 적용한다.
5. `rbac/kember-user-roles.yaml`의 ClusterRole을 조직 group 또는 service account에 namespace `RoleBinding`으로 연결한다.

`kember-platform-admin`만 `WorkerPool`을 변경할 수 있다. `kember-taskrun-developer`는 `TaskRun`만 만들고 취소 요청(`spec.cancel` patch)을 할 수 있다.
