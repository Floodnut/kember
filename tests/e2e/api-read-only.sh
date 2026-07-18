#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kember-e2e}"
API_IMAGE="${API_IMAGE:-kember-api:e2e}"
CACHE_DIR="${PWD}/.cache/e2e-api"
BAZEL="${BAZEL:-bazel}"
BAZEL_OUTPUT_USER_ROOT="${BAZEL_OUTPUT_USER_ROOT:-}"

assert_static_contract() {
  rg -q 'resources: \["workerpools", "taskruns"\]' deploy/rbac/kember-api.yaml
  rg -q 'verbs: \["get", "list"\]' deploy/rbac/kember-api.yaml
  if rg -q 'verbs:.*(watch|create|update|patch|delete)' deploy/rbac/kember-api.yaml; then
    echo "kember-api RBAC is not read-only" >&2
    return 1
  fi
  rg -q 'name: KEMBER_NAMESPACE' deploy/api/api.yaml
  rg -q 'value: kember-system' deploy/api/api.yaml
  rg -q 'targetPort: 8080' deploy/api/api.yaml
}

assert_static_contract
if [[ "${STATIC_ONLY:-false}" == "true" ]]; then
  exit 0
fi

mkdir -p "${CACHE_DIR}"
bazel_args=()
if [[ -n "${BAZEL_OUTPUT_USER_ROOT}" ]]; then
  bazel_args+=("--output_user_root=${BAZEL_OUTPUT_USER_ROOT}")
fi
"${BAZEL}" "${bazel_args[@]}" build //apps/kember-api:kember-api_deploy.jar
cp -fL bazel-bin/apps/kember-api/kember-api_deploy.jar "${CACHE_DIR}/kember-api_deploy.jar"
docker build -t "${API_IMAGE}" -f deploy/api/Dockerfile "${CACHE_DIR}"
if ! kind load docker-image --name "${CLUSTER_NAME}" "${API_IMAGE}"; then
  docker save "${API_IMAGE}" | docker exec -i "${CLUSTER_NAME}-control-plane" ctr -n k8s.io images import -
fi

kubectl apply -f deploy/operator/namespace.yaml
kubectl apply -f deploy/crd/kember.openflood.org_workerpools.yaml
kubectl apply -f deploy/crd/kember.openflood.org_taskruns.yaml
kubectl apply -f deploy/rbac/kember-api.yaml
kubectl apply -f deploy/api/api.yaml
kubectl -n kember-system set image deployment/kember-api "api=${API_IMAGE}"

kubectl apply -f - <<'EOF'
apiVersion: kember.openflood.org/v1alpha1
kind: WorkerPool
metadata:
  name: api-smoke
  namespace: kember-system
spec:
  execution:
    mode: job
  lifecycle:
    profile: runToCompletion
    maxTasksPerWorker: 1
  template:
    image: busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
    command: ["/bin/sh", "-c"]
    argsTemplate: ["echo {{input.ref}}"]
    serviceAccountName: default
    inputPolicy:
      allowedPrefixes: ["smoke://"]
    resources: {}
  taskPolicy:
    queueTimeoutSeconds: 30
    timeoutSeconds: 60
    retentionSeconds: 60
---
apiVersion: kember.openflood.org/v1alpha1
kind: TaskRun
metadata:
  name: api-smoke
  namespace: kember-system
spec:
  workerPoolRef:
    name: api-smoke
  input:
    ref: smoke://input
  timeoutSeconds: 30
EOF

kubectl -n kember-system rollout status deployment/kember-api --timeout=120s
kubectl -n kember-system port-forward service/kember-api 18081:8080 >"${CACHE_DIR}/port-forward.log" 2>&1 &
port_forward_pid=$!
trap 'kill "${port_forward_pid}" >/dev/null 2>&1 || true' EXIT

for _ in $(seq 1 30); do
  if curl --fail --silent http://127.0.0.1:18081/healthz >/dev/null; then
    break
  fi
  sleep 1
done

namespaces="$(curl --fail --silent http://127.0.0.1:18081/api/v1/namespaces)"
worker_pool="$(curl --fail --silent http://127.0.0.1:18081/api/v1/namespaces/kember-system/worker-pools/api-smoke)"
task_run="$(curl --fail --silent http://127.0.0.1:18081/api/v1/namespaces/kember-system/task-runs/api-smoke)"

[[ "${namespaces}" == *'"cluster":"local"'* && "${namespaces}" == *'"name":"kember-system"'* ]]
[[ "${worker_pool}" == *'"namespace":"kember-system"'* && "${worker_pool}" == *'"name":"api-smoke"'* ]]
[[ "${task_run}" == *'"namespace":"kember-system"'* && "${task_run}" == *'"name":"api-smoke"'* ]]
