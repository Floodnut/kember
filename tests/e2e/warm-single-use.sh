#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kember-e2e}"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-kember-operator:e2e}"
WORKER_IMAGE="busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662"
CACHE_DIR="${PWD}/.cache/e2e"

wait_pool_status_ready() {
  local generation status
  generation="$(kubectl -n kember-warm-e2e get workerpool echo-warm -o jsonpath='{.metadata.generation}')"
  for _ in $(seq 1 120); do
    status="$(kubectl -n kember-warm-e2e get workerpool echo-warm -o jsonpath='{.status.observedGeneration}{" "}{.status.capacity.desired}{" "}{.status.capacity.starting}{" "}{.status.capacity.ready}{" "}{.status.capacity.leased}{" "}{.status.capacity.terminating}{" "}{.status.conditions[?(@.type=="Ready")].status}{" "}{.status.conditions[?(@.type=="Progressing")].status}{" "}{.status.conditions[?(@.type=="Degraded")].status}' 2>/dev/null || true)"
    if [[ "${status}" == "${generation} 2 0 2 0 0 True False False" ]]; then
      return 0
    fi
    sleep 1
  done
  echo "WorkerPool status did not converge: ${status}" >&2
  return 1
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

kubectl delete namespace kember-warm-e2e --ignore-not-found --wait=true
kubectl apply -l kember.dev/e2e-stage=pool -f deploy/samples/e2e-warm-single-use.yaml

for _ in $(seq 1 120); do
  ready="$(kubectl -n kember-warm-e2e get pods -l kember.dev/workerpool=echo-warm --field-selector=status.phase=Running -o jsonpath='{range .items[*]}{range .status.conditions[?(@.type=="Ready")]}{.status}{"\n"}{end}{end}' | awk '$1 == "True" {count++} END {print count+0}')"
  if [[ "${ready}" == "2" ]]; then
    break
  fi
  sleep 1
done
[[ "${ready}" == "2" ]]
wait_pool_status_ready

kubectl apply -l kember.dev/e2e-stage=task -f deploy/samples/e2e-warm-single-use.yaml

task_uid="$(kubectl -n kember-warm-e2e get taskrun echo-warm -o jsonpath='{.metadata.uid}')"
lease_holder=""
for _ in $(seq 1 30); do
  lease_holder="$(kubectl -n kember-warm-e2e get leases -l "kember.dev/taskrun-uid=${task_uid}" -o jsonpath='{.items[0].spec.holderIdentity}' 2>/dev/null || true)"
  if [[ "${lease_holder}" == "${task_uid}" ]]; then
    break
  fi
  sleep 0.2
done
[[ "${lease_holder}" == "${task_uid}" ]]

for _ in $(seq 1 120); do
  phase="$(kubectl -n kember-warm-e2e get taskrun echo-warm -o jsonpath='{.status.phase}')"
  if [[ "${phase}" == "Succeeded" ]]; then
    break
  fi
  if [[ "${phase}" == "Failed" || "${phase}" == "TimedOut" || "${phase}" == "Rejected" || "${phase}" == "Cancelled" ]]; then
    kubectl -n kember-warm-e2e get taskrun echo-warm -o yaml
    exit 1
  fi
  sleep 1
done
[[ "${phase}" == "Succeeded" ]]

used_worker="$(kubectl -n kember-warm-e2e get taskrun echo-warm -o jsonpath='{.status.workerRef.name}')"
[[ -n "${used_worker}" ]]
kubectl -n kember-warm-e2e wait --for=delete "pod/${used_worker}" --timeout=60s

for _ in $(seq 1 120); do
  ready="$(kubectl -n kember-warm-e2e get pods -l kember.dev/workerpool=echo-warm --field-selector=status.phase=Running -o jsonpath='{range .items[*]}{range .status.conditions[?(@.type=="Ready")]}{.status}{"\n"}{end}{end}' | awk '$1 == "True" {count++} END {print count+0}')"
  if [[ "${ready}" == "2" ]]; then
    break
  fi
  sleep 1
done
[[ "${ready}" == "2" ]]
wait_pool_status_ready
[[ "$(kubectl -n kember-warm-e2e get jobs -l kember.dev/taskrun-uid -o name)" == "" ]]
[[ "$(kubectl -n kember-warm-e2e get leases -l "kember.dev/taskrun-uid=${task_uid}" -o name)" == "" ]]

kubectl -n kember-warm-e2e patch workerpool echo-warm --type merge -p '{"spec":{"template":{"readinessProbe":{"periodSeconds":2}}}}'
wait_pool_status_ready

kubectl -n kember-warm-e2e get workerpool,pods,taskrun,lease
