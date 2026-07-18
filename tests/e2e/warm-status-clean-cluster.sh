#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kember-e2e}"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-kember-operator:e2e}"
WORKER_IMAGE="busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662"
CACHE_DIR="${PWD}/.cache/e2e"
NAMESPACE="kember-status-e2e"
POOL="status-warm"

pool_status_line() {
  kubectl -n "${NAMESPACE}" get workerpool "${POOL}" -o jsonpath='{.status.observedGeneration}{" "}{.status.capacity.desired}{" "}{.status.capacity.starting}{" "}{.status.capacity.ready}{" "}{.status.capacity.leased}{" "}{.status.capacity.terminating}{" "}{.status.conditions[?(@.type=="Ready")].status}{" "}{.status.conditions[?(@.type=="Progressing")].status}{" "}{.status.conditions[?(@.type=="Degraded")].status}' 2>/dev/null || true
}

wait_pool_status() {
  local expected="$1" attempts="$2" sleep_seconds="$3"
  local status
  for _ in $(seq 1 "${attempts}"); do
    status="$(pool_status_line)"
    if [[ "${status}" == "${expected}" ]]; then
      return 0
    fi
    sleep "${sleep_seconds}"
  done
  echo "WorkerPool status did not match \"${expected}\": got \"${status}\"" >&2
  kubectl -n "${NAMESPACE}" get workerpool "${POOL}" -o yaml >&2
  return 1
}

assert_pool_status_empty() {
  local status
  for _ in $(seq 1 3); do
    status="$(pool_status_line)"
    if [[ -n "${status// /}" ]]; then
      echo "expected empty status while operator is scaled to 0, got: ${status}" >&2
      kubectl -n "${NAMESPACE}" get workerpool "${POOL}" -o yaml >&2
      exit 1
    fi
    sleep 1
  done
}

mkdir -p "${CACHE_DIR}"
GOARCH="$(docker version --format '{{.Server.Arch}}')"
GOWORK=off GOCACHE="${PWD}/.cache/go-build" GOOS=linux GOARCH="${GOARCH}" CGO_ENABLED=0 \
  go build -o "${CACHE_DIR}/kember-operator" ./apps/kember-operator
docker build -t "${OPERATOR_IMAGE}" -f deploy/operator/Dockerfile "${CACHE_DIR}"
docker pull "${WORKER_IMAGE}"
if ! kind load docker-image --name "${CLUSTER_NAME}" "${OPERATOR_IMAGE}"; then
  docker save "${OPERATOR_IMAGE}" | docker exec -i "${CLUSTER_NAME}-control-plane" ctr -n k8s.io images import -
fi
if ! kind load docker-image --name "${CLUSTER_NAME}" "${WORKER_IMAGE}"; then
  docker save "${WORKER_IMAGE}" | docker exec -i "${CLUSTER_NAME}-control-plane" ctr -n k8s.io images import -
fi

kubectl apply -f deploy/operator/namespace.yaml
kubectl apply -f deploy/crd/kember.dev_workerpools.yaml
kubectl apply -f deploy/crd/kember.dev_taskruns.yaml
kubectl apply -f deploy/rbac/kember-operator.yaml
kubectl apply -f deploy/operator/operator.yaml
kubectl -n kember-system rollout restart deployment/kember-operator
kubectl -n kember-system rollout status deployment/kember-operator --timeout=120s

# The kind cluster is reused across e2e scripts, so an operator from a prior
# run may already be reconciling. Scale it to zero so the WorkerPool created
# below starts from a verifiably empty status.
kubectl -n kember-system scale deployment/kember-operator --replicas=0
kubectl -n kember-system wait --for=delete pod -l app.kubernetes.io/name=kember-operator --timeout=60s

kubectl delete namespace "${NAMESPACE}" --ignore-not-found --wait=true
kubectl apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${NAMESPACE}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: status-worker
  namespace: ${NAMESPACE}
---
apiVersion: kember.dev/v1alpha1
kind: WorkerPool
metadata:
  name: ${POOL}
  namespace: ${NAMESPACE}
spec:
  execution:
    mode: exec
    commandTemplate: ["/bin/sh", "-c", "true"]
  lifecycle:
    profile: warmLease
    maxTasksPerWorker: 1
  capacity:
    policy: fixed
    size: 2
  template:
    image: ${WORKER_IMAGE}
    command: ["/bin/sh", "-c"]
    args: ["sleep 6; touch /tmp/kember-ready; exec sleep 3600"]
    serviceAccountName: status-worker
    inputPolicy:
      allowedPrefixes: ["e2e://status/"]
    resources:
      requests:
        cpu: "10m"
        memory: "16Mi"
    readinessProbe:
      exec:
        command: ["/bin/sh", "-c", "test -f /tmp/kember-ready"]
      periodSeconds: 1
  taskPolicy:
    queueTimeoutSeconds: 30
    timeoutSeconds: 60
    retentionSeconds: 60
EOF

# No reconciler is running yet: status must remain completely unset.
assert_pool_status_empty

kubectl -n kember-system scale deployment/kember-operator --replicas=1
kubectl -n kember-system rollout status deployment/kember-operator --timeout=120s

generation="$(kubectl -n "${NAMESPACE}" get workerpool "${POOL}" -o jsonpath='{.metadata.generation}')"

# Readiness is deliberately delayed 6s so the transient Starting window is
# wide enough to observe without flakiness.
wait_pool_status "${generation} 2 2 0 0 0 False True False" 20 0.5

wait_pool_status "${generation} 2 0 2 0 0 True False False" 120 1

kubectl -n "${NAMESPACE}" patch workerpool "${POOL}" --type merge -p '{"spec":{"capacity":{"size":1}}}'
generation="$(kubectl -n "${NAMESPACE}" get workerpool "${POOL}" -o jsonpath='{.metadata.generation}')"
wait_pool_status "${generation} 1 0 1 0 0 True False False" 120 1

kubectl -n "${NAMESPACE}" get workerpool,pods -o wide
