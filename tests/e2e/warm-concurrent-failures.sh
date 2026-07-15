#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

CLUSTER_NAME="${CLUSTER_NAME:-kember-e2e}"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-kember-operator:e2e}"
WORKER_IMAGE="busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662"
CACHE_DIR="${ROOT}/.cache/e2e"
NAMESPACE="kember-warm-failure-e2e"
POOL="failure-warm"
NODE="${CLUSTER_NAME}-control-plane"

mkdir -p "${CACHE_DIR}" "${ROOT}/.cache/go-build"

load_image() {
  local image="$1"
  if ! kind load docker-image --name "${CLUSTER_NAME}" "${image}"; then
    docker save "${image}" | docker exec -i "${NODE}" ctr -n k8s.io images import -
  fi
}

wait_ready_capacity() {
  local wanted="${1:-4}"
  local ready
  for _ in $(seq 1 600); do
    ready="$(kubectl -n "${NAMESPACE}" get pods -l "kember.dev/workerpool=${POOL},!kember.dev/taskrun-uid" -o json | python3 -c '
import json, sys
items = json.load(sys.stdin)["items"]
print(sum(
    item["metadata"].get("deletionTimestamp") is None
    and item.get("status", {}).get("phase") == "Running"
    and any(condition["type"] == "Ready" and condition["status"] == "True" for condition in item.get("status", {}).get("conditions", []))
    for item in items
))
')"
    if [[ "${ready}" == "${wanted}" ]]; then
      return 0
    fi
    sleep 0.2
  done
  echo "timed out waiting for ${wanted} Ready workers" >&2
  return 1
}

wait_no_leases() {
  local leases
  for _ in $(seq 1 600); do
    leases="$(kubectl -n "${NAMESPACE}" get leases -o name)"
    if [[ -z "${leases}" ]]; then
      return 0
    fi
    sleep 0.2
  done
  echo "Lease remained after failure cleanup: ${leases}" >&2
  return 1
}

create_scenario() {
  local scenario="$1"
  local timeout="$2"
  local count="${3:-4}"
  python3 - "${scenario}" "${timeout}" "${count}" <<'PY' | kubectl -n "${NAMESPACE}" create -f - >/dev/null
import json
import sys

scenario, timeout, count = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
items = []
for index in range(1, count + 1):
    items.append({
        "apiVersion": "kember.dev/v1alpha1",
        "kind": "TaskRun",
        "metadata": {
            "name": f"{scenario}-{index}",
            "labels": {"kember.dev/e2e-scenario": scenario},
        },
        "spec": {
            "workerPoolRef": {"name": "failure-warm"},
            "input": {"ref": "e2e://failure/input"},
            "timeoutSeconds": timeout,
        },
    })
print(json.dumps({"apiVersion": "v1", "kind": "List", "items": items}))
PY
}

wait_phase_count() {
  local scenario="$1"
  local phase="$2"
  local wanted="${3:-4}"
  local count
  for _ in $(seq 1 900); do
    count="$(kubectl -n "${NAMESPACE}" get taskruns -l "kember.dev/e2e-scenario=${scenario}" -o jsonpath='{range .items[*]}{.status.phase}{"\n"}{end}' | awk -v phase="${phase}" '$1 == phase {count++} END {print count+0}')"
    if [[ "${count}" == "${wanted}" ]]; then
      return 0
    fi
    sleep 0.2
  done
  kubectl -n "${NAMESPACE}" get taskruns -l "kember.dev/e2e-scenario=${scenario}" -o yaml >&2
  echo "timed out waiting for ${wanted} ${phase} TaskRuns in ${scenario}" >&2
  return 1
}

assert_unique_workers() {
  local scenario="$1"
  kubectl -n "${NAMESPACE}" get taskruns -l "kember.dev/e2e-scenario=${scenario}" -o json | python3 -c '
import json, sys
items = json.load(sys.stdin)["items"]
uids = [item.get("status", {}).get("workerRef", {}).get("uid", "") for item in items]
if len(uids) != 4 or "" in uids or len(set(uids)) != 4:
    raise SystemExit(f"worker assignment is not unique: {uids}")
'
}

GOARCH="$(docker version --format '{{.Server.Arch}}')"
GOWORK=off GOCACHE="${ROOT}/.cache/go-build" GOOS=linux GOARCH="${GOARCH}" CGO_ENABLED=0 \
  go build -o "${CACHE_DIR}/kember-operator" ./apps/kember-operator
docker build -t "${OPERATOR_IMAGE}" -f deploy/operator/Dockerfile "${CACHE_DIR}"
docker pull "${WORKER_IMAGE}"
load_image "${OPERATOR_IMAGE}"
load_image "${WORKER_IMAGE}"

kubectl apply -f deploy/operator/namespace.yaml >/dev/null
kubectl apply -f deploy/crd/kember.dev_workerpools.yaml >/dev/null
kubectl apply -f deploy/crd/kember.dev_taskruns.yaml >/dev/null
kubectl apply -f deploy/rbac/kember-operator.yaml >/dev/null
kubectl apply -f deploy/operator/operator.yaml >/dev/null
kubectl -n kember-system rollout restart deployment/kember-operator >/dev/null
kubectl -n kember-system rollout status deployment/kember-operator --timeout=120s >/dev/null

kubectl delete namespace "${NAMESPACE}" --ignore-not-found --wait=true >/dev/null
kubectl apply -f - >/dev/null <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${NAMESPACE}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: failure-worker
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
    commandTemplate: ["/bin/sh", "-c", "sleep 300"]
  lifecycle:
    profile: warmLease
    maxTasksPerWorker: 1
  capacity:
    policy: fixed
    size: 4
  template:
    image: ${WORKER_IMAGE}
    command: ["/bin/sh", "-c"]
    args: ["exec sleep 3600"]
    serviceAccountName: failure-worker
    inputPolicy:
      allowedPrefixes: ["e2e://failure/"]
    resources:
      requests:
        cpu: 10m
        memory: 16Mi
    readinessProbe:
      exec:
        command: ["/bin/sh", "-c", "true"]
      periodSeconds: 1
  taskPolicy:
    queueTimeoutSeconds: 3
    timeoutSeconds: 60
    retentionSeconds: 60
EOF

wait_ready_capacity

create_scenario timeout 2
wait_phase_count timeout TimedOut
assert_unique_workers timeout
wait_ready_capacity
wait_no_leases

kubectl -n "${NAMESPACE}" patch workerpool "${POOL}" --type merge -p '{"spec":{"capacity":{"policy":"fixed","size":1}}}' >/dev/null
wait_ready_capacity 1

create_scenario cancel 3 2
wait_phase_count cancel Running 1
pending="$(kubectl -n "${NAMESPACE}" get taskruns -l kember.dev/e2e-scenario=cancel -o json | python3 -c '
import json, sys
items = json.load(sys.stdin)["items"]
pending = [item["metadata"]["name"] for item in items if not item.get("status", {}).get("workerRef")]
if len(pending) != 1:
    raise SystemExit(f"expected one unassigned TaskRun, got {pending}")
print(pending[0])
')"
kubectl -n "${NAMESPACE}" patch taskrun "${pending}" --type merge -p '{"spec":{"cancel":true}}' >/dev/null
wait_phase_count cancel Cancelled 1
wait_phase_count cancel TimedOut 1
[[ -z "$(kubectl -n "${NAMESPACE}" get taskrun "${pending}" -o jsonpath='{.status.workerRef.name}')" ]]
wait_ready_capacity 1
wait_no_leases

create_scenario queue 6 2
wait_phase_count queue Running 1
wait_phase_count queue TimedOut 2
queue_timeout_task="$(kubectl -n "${NAMESPACE}" get taskruns -l kember.dev/e2e-scenario=queue -o json | python3 -c '
import json, sys
items = json.load(sys.stdin)["items"]
queued = [item for item in items if item.get("status", {}).get("conditions", [{}])[0].get("reason") == "QueueTimedOut"]
if len(queued) != 1 or queued[0].get("status", {}).get("workerRef"):
    raise SystemExit(f"expected one unassigned QueueTimedOut TaskRun, got {queued}")
print(queued[0]["metadata"]["name"])
')"
[[ -n "${queue_timeout_task}" ]]
queue_reasons="$(kubectl -n "${NAMESPACE}" get taskruns -l kember.dev/e2e-scenario=queue -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Completed")].reason}{"\n"}{end}')"
[[ "$(awk '$1 == "QueueTimedOut" {count++} END {print count+0}' <<<"${queue_reasons}")" == "1" ]]
[[ "$(awk '$1 == "TaskTimedOut" {count++} END {print count+0}' <<<"${queue_reasons}")" == "1" ]]
wait_ready_capacity 1
wait_no_leases

kubectl -n "${NAMESPACE}" patch workerpool "${POOL}" --type merge -p '{"spec":{"capacity":{"policy":"fixed","size":4}}}' >/dev/null
wait_ready_capacity

create_scenario podloss 60
wait_phase_count podloss Running
assert_unique_workers podloss
worker_pods="$(kubectl -n "${NAMESPACE}" get taskruns -l kember.dev/e2e-scenario=podloss -o jsonpath='{range .items[*]}{.status.workerRef.name}{"\n"}{end}')"
kubectl -n "${NAMESPACE}" delete pod ${worker_pods} --grace-period=0 --force --wait=false >/dev/null
wait_phase_count podloss Failed
podloss_reasons="$(kubectl -n "${NAMESPACE}" get taskruns -l kember.dev/e2e-scenario=podloss -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Completed")].reason}{"\n"}{end}')"
[[ "$(awk '$1 == "WorkerLost" {count++} END {print count+0}' <<<"${podloss_reasons}")" == "4" ]]
wait_ready_capacity
wait_no_leases

create_scenario restart 60
wait_phase_count restart Running
assert_unique_workers restart
[[ "$(kubectl -n "${NAMESPACE}" get leases -o name | wc -l | tr -d ' ')" == "4" ]]

kubectl -n kember-system rollout restart deployment/kember-operator >/dev/null
kubectl -n kember-system rollout status deployment/kember-operator --timeout=120s >/dev/null
wait_phase_count restart Failed

reasons="$(kubectl -n "${NAMESPACE}" get taskruns -l kember.dev/e2e-scenario=restart -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Completed")].reason}{"\n"}{end}')"
[[ "$(awk '$1 == "ExecutionOutcomeUnknown" {count++} END {print count+0}' <<<"${reasons}")" == "4" ]]

wait_ready_capacity
wait_no_leases
[[ -z "$(kubectl -n "${NAMESPACE}" get jobs -o name)" ]]

kubectl -n "${NAMESPACE}" get workerpool,pods,taskruns,leases
