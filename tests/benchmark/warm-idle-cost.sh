#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

CLUSTER_NAME="${CLUSTER_NAME:-kember-e2e}"
NAMESPACE="${NAMESPACE:-kember-benchmark}"
POOL="${POOL:-checkov-warm}"
NODE="${NODE:-${CLUSTER_NAME}-control-plane}"
STABILIZE_SECONDS="${STABILIZE_SECONDS:-15}"
SAMPLES="${SAMPLES:-20}"
SAMPLE_INTERVAL_SECONDS="${SAMPLE_INTERVAL_SECONDS:-1}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT}/.cache/benchmark/idle-cost-$(date -u +%Y%m%dT%H%M%SZ)}"

mkdir -p "${OUTPUT_DIR}"

wait_ready_capacity() {
  local wanted="$1"
  local generation ready
  generation="$(kubectl -n "${NAMESPACE}" get workerpool "${POOL}" -o jsonpath='{.metadata.generation}')"
  for _ in $(seq 1 600); do
    ready="$(kubectl -n "${NAMESPACE}" get pods -l "kember.openflood.org/workerpool=${POOL},kember.openflood.org/workerpool-generation=${generation},!kember.openflood.org/taskrun-uid" --field-selector=status.phase=Running -o jsonpath='{range .items[*]}{range .status.conditions[?(@.type=="Ready")]}{.status}{"\n"}{end}{end}' | awk '$1 == "True" {count++} END {print count+0}')"
    if [[ "${ready}" == "${wanted}" ]]; then
      return 0
    fi
    sleep 0.2
  done
  echo "timed out waiting for ${wanted} unassigned Ready workers" >&2
  return 1
}

collect_condition() {
  local size="$1"
  echo "setting fixed capacity to ${size}" >&2
  kubectl -n "${NAMESPACE}" patch workerpool "${POOL}" --type merge -p "{\"spec\":{\"capacity\":{\"policy\":\"fixed\",\"size\":${size}}}}" >/dev/null
  wait_ready_capacity "${size}"
  sleep "${STABILIZE_SECONDS}"

  kubectl -n "${NAMESPACE}" get pods -l "kember.openflood.org/workerpool=${POOL},!kember.openflood.org/taskrun-uid" -o json > "${OUTPUT_DIR}/pods-size${size}.json"
  python3 - "${OUTPUT_DIR}/pods-size${size}.json" "${OUTPUT_DIR}/requests-size${size}.txt" <<'PY'
import json
import sys

source, destination = sys.argv[1:]
pods = json.load(open(source))["items"]
active = [
    pod for pod in pods
    if not pod["metadata"].get("deletionTimestamp")
    and pod.get("status", {}).get("phase") == "Running"
    and any(
        condition.get("type") == "Ready" and condition.get("status") == "True"
        for condition in pod.get("status", {}).get("conditions", [])
    )
]
with open(destination, "w") as output:
    for pod in active:
        resources = pod["spec"]["containers"][0].get("resources", {})
        requests = resources.get("requests", {})
        limits = resources.get("limits", {})
        print(
            pod["metadata"]["name"],
            requests.get("cpu", ""), requests.get("memory", ""),
            limits.get("cpu", "-"), limits.get("memory", "-"),
            file=output,
        )
PY

  for sample in $(seq -w 1 "${SAMPLES}"); do
    docker exec "${NODE}" crictl stats -o json > "${OUTPUT_DIR}/cri-size${size}-${sample}.json"
    sleep "${SAMPLE_INTERVAL_SECONDS}"
  done
}

collect_condition 1
collect_condition 4
python3 tests/benchmark/summarize_idle_cost.py "${OUTPUT_DIR}" | tee "${OUTPUT_DIR}/summary.stdout.json"
echo "idle cost results: ${OUTPUT_DIR}" >&2
