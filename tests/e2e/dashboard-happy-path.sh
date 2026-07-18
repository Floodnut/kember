#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${KEMBER_NAMESPACE:-kember-system}"
API_NAMESPACE="${KEMBER_API_NAMESPACE:-kember-system}"
NAME="${KEMBER_DASHBOARD_FIXTURE_NAME:-dashboard-warm}"
API_URL="${KEMBER_DASHBOARD_API_URL:-http://127.0.0.1:18081}"
WORKER_IMAGE="${WORKER_IMAGE:-busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662}"
CACHE_DIR="${PWD}/.cache/e2e-dashboard"

port_forward_pid=""
cleanup() {
  if [[ -n "${port_forward_pid}" ]]; then
    kill "${port_forward_pid}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

wait_pool_ready() {
  local generation status
  generation="$(kubectl -n "${NAMESPACE}" get workerpool "${NAME}" -o jsonpath='{.metadata.generation}')"
  for _ in $(seq 1 120); do
    status="$(kubectl -n "${NAMESPACE}" get workerpool "${NAME}" -o jsonpath='{.status.observedGeneration}{" "}{.status.capacity.desired}{" "}{.status.capacity.starting}{" "}{.status.capacity.ready}{" "}{.status.capacity.leased}{" "}{.status.capacity.terminating}{" "}{.status.conditions[?(@.type=="Ready")].status}{" "}{.status.conditions[?(@.type=="Progressing")].status}{" "}{.status.conditions[?(@.type=="Degraded")].status}' 2>/dev/null || true)"
    if [[ "${status}" == "${generation} 1 0 1 0 0 True False False" ]]; then
      return 0
    fi
    sleep 1
  done
  echo "WorkerPool ${NAMESPACE}/${NAME} did not become ready: ${status}" >&2
  return 1
}

ensure_api() {
  if curl --fail --silent "${API_URL}/healthz" >/dev/null 2>&1; then
    return 0
  fi

  mkdir -p "${CACHE_DIR}"
  kubectl -n "${API_NAMESPACE}" port-forward service/kember-api 18081:8080 >"${CACHE_DIR}/port-forward.log" 2>&1 &
  port_forward_pid=$!

  for _ in $(seq 1 30); do
    if curl --fail --silent "${API_URL}/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "Kember API did not become reachable at ${API_URL}" >&2
  return 1
}

kubectl -n "${NAMESPACE}" delete taskrun "${NAME}" --ignore-not-found --wait=true
kubectl -n "${NAMESPACE}" delete workerpool "${NAME}" --ignore-not-found --wait=true

kubectl -n "${NAMESPACE}" apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${NAME}-worker
---
apiVersion: kember.openflood.org/v1alpha1
kind: WorkerPool
metadata:
  name: ${NAME}
spec:
  execution:
    mode: exec
    commandTemplate:
      - /bin/sh
      - -c
      - 'sleep 2; test "\$1" = "dashboard://happy-path/input"'
      - kember-task
      - "{{input.ref}}"
  lifecycle:
    profile: warmLease
    maxTasksPerWorker: 1
  capacity:
    policy: fixed
    size: 1
  template:
    image: ${WORKER_IMAGE}
    command: ["/bin/sh", "-c"]
    args: ["touch /tmp/kember-ready; exec sleep 3600"]
    serviceAccountName: ${NAME}-worker
    inputPolicy:
      allowedPrefixes: ["dashboard://happy-path/"]
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
    retentionSeconds: 300
EOF

wait_pool_ready

kubectl -n "${NAMESPACE}" apply -f - <<EOF
apiVersion: kember.openflood.org/v1alpha1
kind: TaskRun
metadata:
  name: ${NAME}
spec:
  workerPoolRef:
    name: ${NAME}
  input:
    ref: dashboard://happy-path/input
  timeoutSeconds: 30
EOF

for _ in $(seq 1 120); do
  phase="$(kubectl -n "${NAMESPACE}" get taskrun "${NAME}" -o jsonpath='{.status.phase}')"
  if [[ "${phase}" == "Succeeded" ]]; then
    break
  fi
  if [[ "${phase}" == "Failed" || "${phase}" == "TimedOut" || "${phase}" == "Rejected" || "${phase}" == "Cancelled" ]]; then
    kubectl -n "${NAMESPACE}" get taskrun "${NAME}" -o yaml
    exit 1
  fi
  sleep 1
done
[[ "${phase}" == "Succeeded" ]]

used_worker="$(kubectl -n "${NAMESPACE}" get taskrun "${NAME}" -o jsonpath='{.status.workerRef.name}')"
[[ -n "${used_worker}" ]]
kubectl -n "${NAMESPACE}" wait --for=delete "pod/${used_worker}" --timeout=60s
wait_pool_ready

ensure_api
worker_pool="$(curl --fail --silent "${API_URL}/api/v1/namespaces/${NAMESPACE}/worker-pools/${NAME}")"
task_run="$(curl --fail --silent "${API_URL}/api/v1/namespaces/${NAMESPACE}/task-runs/${NAME}")"
ui_route="$(curl --fail --silent "${API_URL}/worker-pools/${NAME}")"

[[ "${worker_pool}" == *'"name":"'"${NAME}"'"'* ]]
[[ "${worker_pool}" == *'"ready":1'* ]]
[[ "${task_run}" == *'"phase":"Succeeded"'* ]]
[[ "${task_run}" == *'"workerPool":"'"${NAME}"'"'* ]]
[[ "${ui_route}" == *'<div id="root"></div>'* ]]

kubectl -n "${NAMESPACE}" get workerpool "${NAME}"
kubectl -n "${NAMESPACE}" get taskrun "${NAME}"
