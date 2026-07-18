#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

CLUSTER_NAME="${CLUSTER_NAME:-kember-e2e}"
CHECKOV_TAG="${CHECKOV_TAG:-bridgecrew/checkov:3.3.7}"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-kember-operator:e2e}"
WARMUP_REPETITIONS="${WARMUP_REPETITIONS:-1}"
REPETITIONS="${REPETITIONS:-5}"
EXPERIMENT="${EXPERIMENT:-burst}"
TASKS_PER_CONDITION="${TASKS_PER_CONDITION:-4}"
ARRIVAL_INTERVALS="${ARRIVAL_INTERVALS:-0 1 5}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT}/.cache/benchmark/burst-$(date -u +%Y%m%dT%H%M%SZ)}"
CACHE_DIR="${ROOT}/.cache/e2e"
NAMESPACE="${NAMESPACE:-kember-burst-$(date -u +%H%M%S)}"
POOL="checkov-warm"
NODE="${CLUSTER_NAME}-control-plane"

mkdir -p "${OUTPUT_DIR}" "${CACHE_DIR}" "${ROOT}/.cache/go-build"
TASK_RESULTS="${OUTPUT_DIR}/tasks.csv"
BURST_RESULTS="${OUTPUT_DIR}/bursts.csv"
printf 'pool_size,concurrency,arrival_interval_seconds,repetition,task_name,outcome,worker_name,worker_uid,lease_name,created_at_ms,dispatched_at_ms,completed_at_ms,queue_wait_ms,active_duration_ms,task_e2e_ms\n' > "${TASK_RESULTS}"
printf 'pool_size,concurrency,arrival_interval_seconds,repetition,makespan_ms,throughput_tasks_per_second,observed_max_parallel,reserved_worker_seconds,reserved_cpu_core_seconds,reserved_memory_mib_seconds\n' > "${BURST_RESULTS}"

now_ms() {
  python3 -c 'import time; print(time.time_ns() // 1_000_000)'
}

load_image() {
  local image="$1"
  if ! kind load docker-image --name "${CLUSTER_NAME}" "${image}"; then
    docker save "${image}" | docker exec -i "${NODE}" ctr -n k8s.io images import -
  fi
}

wait_ready_capacity() {
  local wanted="$1"
  local generation ready
  generation="$(kubectl -n "${NAMESPACE}" get workerpool "${POOL}" -o jsonpath='{.metadata.generation}')"
  for _ in $(seq 1 900); do
    ready="$(kubectl -n "${NAMESPACE}" get pods -l "kember.dev/workerpool=${POOL},kember.dev/workerpool-generation=${generation},!kember.dev/taskrun-uid" --field-selector=status.phase=Running -o jsonpath='{range .items[*]}{range .status.conditions[?(@.type=="Ready")]}{.status}{"\n"}{end}{end}' | awk '$1 == "True" {count++} END {print count+0}')"
    if [[ "${ready}" == "${wanted}" ]]; then
      return 0
    fi
    sleep 0.2
  done
  echo "timed out waiting for ${wanted} unassigned Ready workers" >&2
  return 1
}

wait_no_leases() {
  local remaining
  for _ in $(seq 1 600); do
    remaining="$(kubectl -n "${NAMESPACE}" get leases -o name)"
    if [[ -z "${remaining}" ]]; then
      return 0
    fi
    sleep 0.2
  done
  echo "Lease remained after worker drain timeout: ${remaining}" >&2
  return 1
}

set_pool_size() {
  local size="$1"
  kubectl -n "${NAMESPACE}" patch workerpool "${POOL}" --type merge -p "{\"spec\":{\"capacity\":{\"policy\":\"fixed\",\"size\":${size}}}}" >/dev/null
  wait_ready_capacity "${size}"
}

create_burst() {
  local scenario="$1"
  local concurrency="$2"
  local arrival_interval="$3"
  local index
  if [[ "${arrival_interval}" == "0" ]]; then
    python3 - "${scenario}" "${concurrency}" <<'PY' | kubectl -n "${NAMESPACE}" create -f - >/dev/null
import json
import sys

scenario, concurrency = sys.argv[1], int(sys.argv[2])
items = []
for index in range(1, concurrency + 1):
    items.append({
        "apiVersion": "kember.dev/v1alpha1",
        "kind": "TaskRun",
        "metadata": {
            "name": f"{scenario}-{index:02d}",
            "labels": {"kember.dev/benchmark-scenario": scenario},
        },
        "spec": {
            "workerPoolRef": {"name": "checkov-warm"},
            "input": {"ref": "bench://fixture/main.tf"},
            "timeoutSeconds": 120,
        },
    })
print(json.dumps({"apiVersion": "v1", "kind": "List", "items": items}))
PY
    return 0
  fi

  for index in $(seq 1 "${concurrency}"); do
    python3 - "${scenario}" "${index}" <<'PY' | kubectl -n "${NAMESPACE}" create -f - >/dev/null
import json
import sys

scenario, index = sys.argv[1], int(sys.argv[2])
print(json.dumps({
    "apiVersion": "kember.dev/v1alpha1",
    "kind": "TaskRun",
    "metadata": {
        "name": f"{scenario}-{index:02d}",
        "labels": {"kember.dev/benchmark-scenario": scenario},
    },
    "spec": {
        "workerPoolRef": {"name": "checkov-warm"},
        "input": {"ref": "bench://fixture/main.tf"},
        "timeoutSeconds": 120,
    },
}))
PY
    if [[ "${index}" != "${concurrency}" ]]; then
      sleep "${arrival_interval}"
    fi
  done
}

wait_burst() {
  local scenario="$1"
  local concurrency="$2"
  local phases total terminal
  for _ in $(seq 1 3600); do
    phases="$(kubectl -n "${NAMESPACE}" get taskruns -l "kember.dev/benchmark-scenario=${scenario}" -o jsonpath='{range .items[*]}{.status.phase}{"\n"}{end}' 2>/dev/null || true)"
    total="$(awk 'NF {count++} END {print count+0}' <<<"${phases}")"
    terminal="$(awk '$1 ~ /^(Succeeded|Failed|TimedOut|Rejected|Cancelled)$/ {count++} END {print count+0}' <<<"${phases}")"
    if [[ "${total}" == "${concurrency}" && "${terminal}" == "${concurrency}" ]]; then
      return 0
    fi
    sleep 0.1
  done
  echo "timed out waiting for burst ${scenario}" >&2
  return 1
}

record_burst() {
  local pool_size="$1"
  local concurrency="$2"
  local repetition="$3"
  local arrival_interval="$4"
  local scenario="$5"
  local makespan="$6"
  local snapshot="${OUTPUT_DIR}/${scenario}.json"
  kubectl -n "${NAMESPACE}" get taskruns -l "kember.dev/benchmark-scenario=${scenario}" -o json > "${snapshot}"
  python3 - "${snapshot}" "${TASK_RESULTS}" "${BURST_RESULTS}" "${pool_size}" "${concurrency}" "${arrival_interval}" "${repetition}" "${makespan}" <<'PY'
import csv
import datetime
import json
import sys

snapshot, task_path, burst_path, pool_size, concurrency, arrival_interval, repetition, makespan = sys.argv[1:]

def millis(value):
    return int(datetime.datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp() * 1000)

with open(snapshot) as source:
    items = json.load(source)["items"]

intervals = []
task_rows = []
for item in items:
    metadata, status = item["metadata"], item.get("status", {})
    created = millis(metadata["creationTimestamp"])
    dispatched = millis(status["dispatchedAt"])
    completed = millis(status["completedAt"])
    worker = status.get("workerRef", {})
    intervals.append((dispatched, 1))
    intervals.append((completed, -1))
    task_rows.append([
        pool_size, concurrency, arrival_interval, repetition, metadata["name"], status.get("phase", ""),
        worker.get("name", ""), worker.get("uid", ""), worker.get("leaseName", ""),
        created, dispatched, completed, dispatched-created, completed-dispatched, completed-created,
    ])

active = maximum = 0
for _, delta in sorted(intervals, key=lambda point: (point[0], point[1])):
    active += delta
    maximum = max(maximum, active)

with open(task_path, "a", newline="") as output:
    csv.writer(output).writerows(task_rows)
with open(burst_path, "a", newline="") as output:
    csv.writer(output).writerow([
        pool_size, concurrency, arrival_interval, repetition, makespan,
        f"{int(concurrency) / (int(makespan) / 1000):.6f}", maximum,
        f"{int(pool_size) * int(makespan) / 1000:.3f}",
        f"{int(pool_size) * int(makespan) / 1000 * 0.1:.3f}",
        f"{int(pool_size) * int(makespan) / 1000 * 256:.3f}",
    ])

failures = [row[4] for row in task_rows if row[5] != "Succeeded"]
worker_uids = [row[7] for row in task_rows]
if failures:
    raise SystemExit(f"terminal failures: {failures}")
if len(worker_uids) != len(set(worker_uids)):
    raise SystemExit("a worker was assigned to more than one TaskRun in a burst")
PY
}

run_burst() {
  local pool_size="$1"
  local concurrency="$2"
  local repetition="$3"
  local record="$4"
  local arrival_interval="${5:-0}"
  local interval_label="${arrival_interval/./p}"
  local scenario="p${pool_size}-c${concurrency}-i${interval_label}-${repetition}"
  local start end
  start="$(now_ms)"
  create_burst "${scenario}" "${concurrency}" "${arrival_interval}"
  wait_burst "${scenario}" "${concurrency}"
  end="$(now_ms)"
  if [[ "${record}" == "1" ]]; then
    record_burst "${pool_size}" "${concurrency}" "${repetition}" "${arrival_interval}" "${scenario}" "$((end-start))"
  fi
  wait_ready_capacity "${pool_size}"
}

GOARCH="$(docker version --format '{{.Server.Arch}}')"
GOWORK=off GOCACHE="${ROOT}/.cache/go-build" GOOS=linux GOARCH="${GOARCH}" CGO_ENABLED=0 \
  go build -o "${CACHE_DIR}/kember-operator" ./apps/kember-operator
docker build -t "${OPERATOR_IMAGE}" -f deploy/operator/Dockerfile "${CACHE_DIR}"
docker pull "${CHECKOV_TAG}"
load_image "${OPERATOR_IMAGE}"
load_image "${CHECKOV_TAG}"

NODE_DIGEST="$(docker exec "${NODE}" ctr -n k8s.io images ls | awk -v ref="docker.io/${CHECKOV_TAG}" '$1 == ref {digest=$3} END {print digest}')"
if [[ ! "${NODE_DIGEST}" =~ ^sha256:[a-f0-9]{64}$ ]]; then
  echo "failed to resolve node Checkov manifest digest: ${NODE_DIGEST}" >&2
  exit 1
fi
CHECKOV_IMAGE="bridgecrew/checkov@${NODE_DIGEST}"
docker exec "${NODE}" ctr -n k8s.io images tag "docker.io/${CHECKOV_TAG}" "docker.io/${CHECKOV_IMAGE}" >/dev/null 2>&1 || true

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
  name: benchmark-worker
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
    commandTemplate:
      - /bin/sh
      - -c
      - |
        rm -rf /tmp/kember-fixture
        mkdir -p /tmp/kember-fixture
        printf 'resource "aws_s3_bucket" "example" { bucket = "kember-benchmark" }\n' > /tmp/kember-fixture/main.tf
        exec checkov -d /tmp/kember-fixture --framework terraform --compact --quiet --soft-fail
  lifecycle:
    profile: warmLease
    maxTasksPerWorker: 1
  capacity:
    policy: fixed
    size: 1
  template:
    image: ${CHECKOV_IMAGE}
    command: ["/bin/sh", "-c"]
    args: ["exec sleep 3600"]
    serviceAccountName: benchmark-worker
    inputPolicy:
      allowedPrefixes: ["bench://fixture/"]
    resources:
      requests:
        cpu: 100m
        memory: 256Mi
    readinessProbe:
      exec:
        command: ["/bin/sh", "-c", "command -v checkov >/dev/null"]
      periodSeconds: 1
  taskPolicy:
    queueTimeoutSeconds: 120
    timeoutSeconds: 120
    retentionSeconds: 600
EOF

if [[ "${EXPERIMENT}" == "arrival" ]]; then
  for ((i=1; i<=REPETITIONS; i++)); do
    repetition="$(printf '%02d' "${i}")"
    if (( i % 2 == 1 )); then
      capacities=(1 2 4)
    else
      capacities=(4 2 1)
    fi
    for arrival_interval in ${ARRIVAL_INTERVALS}; do
      for pool_size in "${capacities[@]}"; do
        set_pool_size "${pool_size}"
        run_burst "${pool_size}" "${TASKS_PER_CONDITION}" "${repetition}" 1 "${arrival_interval}"
        echo "completed interval=${arrival_interval}s pool=${pool_size} repetition=${i}/${REPETITIONS}"
      done
    done
  done
  python3 tests/benchmark/summarize_arrival.py "${TASK_RESULTS}" "${BURST_RESULTS}" "${OUTPUT_DIR}/summary.json" | tee "${OUTPUT_DIR}/summary.txt"
  set_pool_size 1
else
  for pair in '1 1' '4 1' '1 4' '4 4' '1 8' '4 8'; do
    read -r pool_size concurrency <<<"${pair}"
    set_pool_size "${pool_size}"
    for ((i=1; i<=WARMUP_REPETITIONS; i++)); do
      run_burst "${pool_size}" "${concurrency}" "warmup-$(printf '%02d' "${i}")" 0 0
    done
    for ((i=1; i<=REPETITIONS; i++)); do
      repetition="$(printf '%02d' "${i}")"
      run_burst "${pool_size}" "${concurrency}" "${repetition}" 1 0
      echo "completed pool=${pool_size} concurrency=${concurrency} repetition=${i}/${REPETITIONS}"
    done
  done
  python3 tests/benchmark/summarize_burst.py "${TASK_RESULTS}" "${BURST_RESULTS}" "${OUTPUT_DIR}/summary.json" | tee "${OUTPUT_DIR}/summary.txt"
  wait_ready_capacity 4
fi

wait_no_leases
echo "benchmark output: ${OUTPUT_DIR}"
