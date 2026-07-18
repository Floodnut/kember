#!/usr/bin/env bash
set -euo pipefail

OPERATOR_IMAGE="${OPERATOR_IMAGE:-kember-operator:e2e}"
NAMESPACE="kember-e2e"

wait_task_succeeded() {
	local phase=""
	for _ in $(seq 1 90); do
		phase="$(kubectl -n "${NAMESPACE}" get taskrun echo -o jsonpath='{.status.phase}' 2>/dev/null || true)"
		if [[ "${phase}" == "Succeeded" ]]; then
			return 0
		fi
		if [[ "${phase}" == "Failed" || "${phase}" == "TimedOut" || "${phase}" == "Rejected" || "${phase}" == "Cancelled" ]]; then
			kubectl -n "${NAMESPACE}" get taskrun echo -o yaml >&2
			return 1
		fi
		sleep 1
	done
	kubectl -n "${NAMESPACE}" get taskrun echo -o yaml >&2 || true
	return 1
}

cleanup() {
	kubectl delete namespace "${NAMESPACE}" --ignore-not-found --wait=true >/dev/null 2>&1 || true
}
trap cleanup EXIT

KEMBER_OPERATOR_IMAGE="${OPERATOR_IMAGE}" ./deploy/install.sh
first_generation="$(kubectl -n kember-system get deployment kember-operator -o jsonpath='{.metadata.generation}')"
KEMBER_OPERATOR_IMAGE="${OPERATOR_IMAGE}" ./deploy/install.sh
second_generation="$(kubectl -n kember-system get deployment kember-operator -o jsonpath='{.metadata.generation}')"
[[ "${second_generation}" == "${first_generation}" ]]

kubectl apply -f deploy/samples/e2e-success.yaml >/dev/null
wait_task_succeeded
job_name="$(kubectl -n "${NAMESPACE}" get taskrun echo -o jsonpath='{.status.jobRef.name}')"
[[ -n "${job_name}" ]]
job_count="$(kubectl -n "${NAMESPACE}" get jobs -l kember.dev/taskrun-uid -o name | wc -l | tr -d ' ')"
[[ "${job_count}" == "1" ]]

kubectl -n kember-system rollout restart deployment/kember-operator >/dev/null
kubectl -n kember-system rollout status deployment/kember-operator --timeout=120s >/dev/null
[[ "$(kubectl -n "${NAMESPACE}" get taskrun echo -o jsonpath='{.status.phase}')" == "Succeeded" ]]
[[ "$(kubectl -n "${NAMESPACE}" get jobs -l kember.dev/taskrun-uid -o name | wc -l | tr -d ' ')" == "1" ]]
[[ "$(kubectl -n "${NAMESPACE}" get taskrun echo -o jsonpath='{.status.jobRef.name}')" == "${job_name}" ]]

echo "operator operations smoke passed"
