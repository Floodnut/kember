#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

CLUSTER_NAME="${CLUSTER_NAME:-kember-e2e}"
WORKLOAD_NAME="${WORKLOAD_NAME:-checkov}"
WORKLOAD_IMAGE_TAG="${WORKLOAD_IMAGE_TAG:-${CHECKOV_TAG:-bridgecrew/checkov:3.3.7}}"
WORKLOAD_IMAGE_REPOSITORY="${WORKLOAD_IMAGE_REPOSITORY:-bridgecrew/checkov}"
WORKLOAD_INPUT_REF="${WORKLOAD_INPUT_REF:-bench://fixture/main.tf}"
WORKLOAD_ALLOWED_PREFIX="${WORKLOAD_ALLOWED_PREFIX:-bench://fixture/}"
WORKLOAD_COMMAND="${WORKLOAD_COMMAND:-rm -rf /tmp/kember-fixture && mkdir -p /tmp/kember-fixture && printf 'resource \"aws_s3_bucket\" \"example\" { bucket = \"kember-benchmark\" }\n' > /tmp/kember-fixture/main.tf && exec checkov -d /tmp/kember-fixture --framework terraform --compact --quiet --soft-fail}"
WORKLOAD_READINESS_COMMAND="${WORKLOAD_READINESS_COMMAND:-command -v checkov >/dev/null}"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-kember-operator:e2e}"
WARMUP_ITERATIONS="${WARMUP_ITERATIONS:-3}"
ITERATIONS="${ITERATIONS:-30}"
TIMING_TOLERANCE_MS="${TIMING_TOLERANCE_MS:-1500}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT}/.cache/benchmark/$(date -u +%Y%m%dT%H%M%SZ)}"
CACHE_DIR="${ROOT}/.cache/e2e"
NAMESPACE="${NAMESPACE:-kember-benchmark}"
NODE="${CLUSTER_NAME}-control-plane"

mkdir -p "${OUTPUT_DIR}" "${CACHE_DIR}" "${ROOT}/.cache/go-build"
RESULTS="${OUTPUT_DIR}/results.csv"
SUMMARY="${OUTPUT_DIR}/summary.json"
printf 'mode,iteration,duration_ms,outcome,resource_name,harness_assignment_ms,harness_active_ms,status_assignment_ms,status_active_ms,assignment_delta_ms,active_delta_ms,timing_tolerance_ms,timing_consistent\n' > "${RESULTS}"

now_ms() {
  python3 -c 'import time; print(time.time_ns() // 1_000_000)'
}

load_image() {
  local image="$1"
  if ! kind load docker-image --name "${CLUSTER_NAME}" "${image}"; then
    docker save "${image}" | docker exec -i "${NODE}" ctr -n k8s.io images import -
  fi
}

wait_unassigned_ready_worker() {
  local ready="0"
  for _ in $(seq 1 900); do
    ready="$(kubectl -n "${NAMESPACE}" get pods -l "kember.dev/workerpool=${WORKLOAD_NAME}-warm,!kember.dev/taskrun-uid" --field-selector=status.phase=Running -o jsonpath='{range .items[*]}{range .status.conditions[?(@.type=="Ready")]}{.status}{"\n"}{end}{end}' | awk '$1 == "True" {count++} END {print count+0}')"
    if [[ "${ready}" -ge 1 ]]; then
      return 0
    fi
    sleep 0.2
  done
  echo "timed out waiting for an unassigned Ready worker" >&2
  return 1
}

wait_taskrun_timing() {
  local name="$1"
  local observed_dispatch_ms observed_terminal_ms phase
  observed_dispatch_ms="$(wait_taskrun_status_field "${name}" dispatchedAt)" || {
    printf 'ObservationTimedOut|0|%s' "$(now_ms)"
    return 0
  }
  observed_terminal_ms="$(wait_taskrun_status_field "${name}" completedAt)" || {
    printf 'ObservationTimedOut|%s|%s' "${observed_dispatch_ms}" "$(now_ms)"
    return 0
  }
  phase="$(kubectl -n "${NAMESPACE}" get taskrun "${name}" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  printf '%s|%s|%s' "${phase}" "${observed_dispatch_ms}" "${observed_terminal_ms}"
}

wait_taskrun_status_field() {
  local name="$1" field="$2"
  kubectl -n "${NAMESPACE}" wait --for="jsonpath={.status.${field}}" --timeout=180s "taskrun/${name}" >/dev/null
  now_ms
}

wait_raw_job() {
  local name="$1"
  local succeeded=""
  local failed=""
  for _ in $(seq 1 1800); do
    succeeded="$(kubectl -n "${NAMESPACE}" get job "${name}" -o jsonpath='{.status.succeeded}' 2>/dev/null || true)"
    failed="$(kubectl -n "${NAMESPACE}" get job "${name}" -o jsonpath='{.status.failed}' 2>/dev/null || true)"
    if [[ "${succeeded}" == "1" ]]; then
      printf 'Succeeded'
      return 0
    fi
    if [[ -n "${failed}" && "${failed}" != "0" ]]; then
      printf 'Failed'
      return 0
    fi
    sleep 0.1
  done
  printf 'ObservationTimedOut'
}

run_raw_job() {
  local iteration="$1"
  local record="$2"
  local name="raw-job-${iteration}"
  local start end outcome
  start="$(now_ms)"
  kubectl -n "${NAMESPACE}" create -f - >/dev/null <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${name}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 600
  template:
    spec:
      restartPolicy: Never
      serviceAccountName: benchmark-worker
      containers:
        - name: worker
          image: ${WORKLOAD_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["/bin/sh", "-c"]
          args:
            - |
              ${WORKLOAD_COMMAND}
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
EOF
  outcome="$(wait_raw_job "${name}")"
  end="$(now_ms)"
  if [[ "${record}" == "1" ]]; then
    printf 'raw-job,%s,%s,%s,%s,,,,,,,,\n' "${iteration}" "$((end-start))" "${outcome}" "${name}" >> "${RESULTS}"
  fi
  if [[ "${outcome}" != "Succeeded" ]]; then
    kubectl -n "${NAMESPACE}" get job "${name}" -o yaml >&2
    kubectl -n "${NAMESPACE}" logs "job/${name}" >&2 || true
    return 1
  fi
}

run_taskrun() {
  local mode="$1"
  local pool="$2"
  local iteration="$3"
  local record="$4"
  local name="${mode}-${iteration}"
  local start end outcome observed_dispatch timing
  if [[ "${mode}" == "warm-lease" ]]; then
    wait_unassigned_ready_worker
  fi
  start="$(now_ms)"
  kubectl -n "${NAMESPACE}" create -f - >/dev/null <<EOF
apiVersion: kember.dev/v1alpha1
kind: TaskRun
metadata:
  name: ${name}
spec:
  workerPoolRef:
    name: ${pool}
  input:
    ref: ${WORKLOAD_INPUT_REF}
  timeoutSeconds: 120
EOF
  IFS='|' read -r outcome observed_dispatch end <<<"$(wait_taskrun_timing "${name}")"
  if [[ "${record}" == "1" ]]; then
    if [[ "${outcome}" == "Succeeded" ]]; then
      timing="$(kubectl -n "${NAMESPACE}" get taskrun "${name}" -o json | python3 tests/benchmark/lifecycle_timing.py "${start}" "${observed_dispatch}" "${end}" "${TIMING_TOLERANCE_MS}")"
      printf '%s,%s,%s,%s,%s,%s\n' "${mode}" "${iteration}" "$((end-start))" "${outcome}" "${name}" "${timing}" >> "${RESULTS}"
    else
      printf '%s,%s,%s,%s,%s,,,,,,,,\n' "${mode}" "${iteration}" "$((end-start))" "${outcome}" "${name}" >> "${RESULTS}"
    fi
  fi
  if [[ "${outcome}" != "Succeeded" ]]; then
    kubectl -n "${NAMESPACE}" get taskrun "${name}" -o yaml >&2
    return 1
  fi
}

capture_lifecycle_metrics() {
  local output="${OUTPUT_DIR}/metrics.txt"
  local names=(
    kember_workerpool_ready_workers
    kember_workerpool_leased_workers
    kember_taskrun_assignment_wait_seconds
    kember_taskrun_active_duration_seconds
    kember_taskrun_total
    kember_worker_termination_requests_total
  )
  kubectl -n kember-system exec deployment/kember-operator -- wget -qO- http://127.0.0.1:8080/metrics > "${output}"
  for name in "${names[@]}"; do
    if ! grep -q "^# TYPE ${name} " "${output}"; then
      echo "missing lifecycle metric: ${name}" >&2
      return 1
    fi
  done
}

run_mode() {
  local mode="$1"
  local iteration="$2"
  local record="$3"
  case "${mode}" in
    raw-job) run_raw_job "${iteration}" "${record}" ;;
    kember-job) run_taskrun "kember-job" "${WORKLOAD_NAME}-job" "${iteration}" "${record}" ;;
    warm-lease) run_taskrun "warm-lease" "${WORKLOAD_NAME}-warm" "${iteration}" "${record}" ;;
    *) echo "unknown mode: ${mode}" >&2; return 2 ;;
  esac
}

GOARCH="$(docker version --format '{{.Server.Arch}}')"
GOWORK=off GOCACHE="${ROOT}/.cache/go-build" GOOS=linux GOARCH="${GOARCH}" CGO_ENABLED=0 \
  go build -o "${CACHE_DIR}/kember-operator" ./apps/kember-operator
docker build -t "${OPERATOR_IMAGE}" -f deploy/operator/Dockerfile "${CACHE_DIR}"
docker pull "${WORKLOAD_IMAGE_TAG}"
load_image "${OPERATOR_IMAGE}"
load_image "${WORKLOAD_IMAGE_TAG}"

NODE_DIGEST="$(docker exec "${NODE}" ctr -n k8s.io images ls | awk -v ref="docker.io/${WORKLOAD_IMAGE_TAG}" '$1 == ref {digest=$3} END {print digest}')"
if [[ ! "${NODE_DIGEST}" =~ ^sha256:[a-f0-9]{64}$ ]]; then
  echo "failed to resolve node ${WORKLOAD_NAME} manifest digest: ${NODE_DIGEST}" >&2
  exit 1
fi
WORKLOAD_IMAGE="${WORKLOAD_IMAGE_REPOSITORY}@${NODE_DIGEST}"
docker exec "${NODE}" ctr -n k8s.io images tag "docker.io/${WORKLOAD_IMAGE_TAG}" "docker.io/${WORKLOAD_IMAGE}" >/dev/null 2>&1 || true
if ! docker exec "${NODE}" crictl inspecti "docker.io/${WORKLOAD_IMAGE}" >/dev/null; then
  echo "node CRI cannot resolve pre-pulled ${WORKLOAD_NAME} digest alias: ${WORKLOAD_IMAGE}" >&2
  exit 1
fi
SOURCE_DIGEST="$(docker image inspect "${WORKLOAD_IMAGE_TAG}" --format '{{index .RepoDigests 0}}')"

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
  name: ${WORKLOAD_NAME}-job
  namespace: ${NAMESPACE}
spec:
  execution:
    mode: job
  template:
    image: ${WORKLOAD_IMAGE}
    command: ["/bin/sh", "-c"]
    argsTemplate:
      - |
        ${WORKLOAD_COMMAND}
    serviceAccountName: benchmark-worker
    inputPolicy:
      allowedPrefixes: ["${WORKLOAD_ALLOWED_PREFIX}"]
    resources:
      requests:
        cpu: 100m
        memory: 256Mi
  taskPolicy:
    queueTimeoutSeconds: 120
    timeoutSeconds: 120
    retentionSeconds: 600
---
apiVersion: kember.dev/v1alpha1
kind: WorkerPool
metadata:
  name: ${WORKLOAD_NAME}-warm
  namespace: ${NAMESPACE}
spec:
  execution:
    mode: exec
    commandTemplate:
      - /bin/sh
      - -c
      - |
        ${WORKLOAD_COMMAND}
  lifecycle:
    profile: warmLease
    maxTasksPerWorker: 1
  capacity:
    policy: fixed
    size: 1
  template:
    image: ${WORKLOAD_IMAGE}
    command: ["/bin/sh", "-c"]
    args: ["exec sleep 3600"]
    serviceAccountName: benchmark-worker
    inputPolicy:
      allowedPrefixes: ["${WORKLOAD_ALLOWED_PREFIX}"]
    resources:
      requests:
        cpu: 100m
        memory: 256Mi
    readinessProbe:
      exec:
        command: ["/bin/sh", "-c", "${WORKLOAD_READINESS_COMMAND}"]
      periodSeconds: 1
  taskPolicy:
    queueTimeoutSeconds: 120
    timeoutSeconds: 120
    retentionSeconds: 600
EOF

wait_unassigned_ready_worker

python3 - "${OUTPUT_DIR}/metadata.json" "${WORKLOAD_NAME}" "${SOURCE_DIGEST}" "${WORKLOAD_IMAGE}" "${WARMUP_ITERATIONS}" "${ITERATIONS}" "${TIMING_TOLERANCE_MS}" <<'PY'
import json
import platform
import subprocess
import sys

def command(*args):
    return subprocess.run(args, check=False, capture_output=True, text=True).stdout.strip()

path, workload_name, source_digest, node_image, warmups, iterations, timing_tolerance = sys.argv[1:]
metadata = {
    "workload_name": workload_name,
    "source_image_digest": source_digest,
    "node_image": node_image,
    "warmup_iterations": int(warmups),
    "measured_iterations": int(iterations),
    "timing_tolerance_ms": int(timing_tolerance),
    "host": platform.platform(),
    "kind_version": command("kind", "version"),
    "kubectl_client": command("kubectl", "version", "--client"),
    "kubectl_client_and_server": command("kubectl", "version"),
    "docker_server_arch": command("docker", "version", "--format", "{{.Server.Arch}}"),
}
with open(path, "w") as output:
    json.dump(metadata, output, indent=2)
    output.write("\n")
PY

echo "running ${WARMUP_ITERATIONS} warm-up iterations per mode"
for ((i=1; i<=WARMUP_ITERATIONS; i++)); do
  iteration="$(printf '%03d' "${i}")"
  run_mode raw-job "warmup-${iteration}" 0
  run_mode kember-job "warmup-${iteration}" 0
  run_mode warm-lease "warmup-${iteration}" 0
done

echo "running ${ITERATIONS} measured iterations per mode"
for ((i=1; i<=ITERATIONS; i++)); do
  iteration="$(printf '%03d' "${i}")"
  case $((i % 3)) in
    1) order=(raw-job kember-job warm-lease) ;;
    2) order=(warm-lease raw-job kember-job) ;;
    0) order=(kember-job warm-lease raw-job) ;;
  esac
  for mode in "${order[@]}"; do
    run_mode "${mode}" "${iteration}" 1
  done
  echo "completed measured iteration ${iteration}/${ITERATIONS}"
done

capture_lifecycle_metrics
python3 tests/benchmark/summarize.py "${RESULTS}" "${SUMMARY}" | tee "${OUTPUT_DIR}/summary.txt"
echo "benchmark output: ${OUTPUT_DIR}"
