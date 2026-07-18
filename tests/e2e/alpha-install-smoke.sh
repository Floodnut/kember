#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-kember-e2e}"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-kember-operator:e2e}"
WORKER_IMAGE="busybox@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662"
CACHE_DIR="${PWD}/.cache/e2e"
METRICS_PORT="${METRICS_PORT:-18080}"

wait_task_phase() {
	local namespace="$1" name="$2" expected="$3" phase=""
	for _ in $(seq 1 120); do
		phase="$(kubectl -n "${namespace}" get taskrun "${name}" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
		if [[ "${phase}" == "${expected}" ]]; then
			return 0
		fi
		if [[ "${phase}" == "Failed" || "${phase}" == "TimedOut" || "${phase}" == "Rejected" || "${phase}" == "Cancelled" ]]; then
			kubectl -n "${namespace}" get taskrun "${name}" -o yaml >&2
			return 1
		fi
		sleep 1
	done
	echo "TaskRun ${namespace}/${name} did not become ${expected}: ${phase}" >&2
	kubectl -n "${namespace}" get taskrun "${name}" -o yaml >&2 || true
	return 1
}

wait_warm_ready() {
	local ready=""
	for _ in $(seq 1 120); do
		ready="$(kubectl -n kember-warm-e2e get pods -l kember.dev/workerpool=echo-warm --field-selector=status.phase=Running -o jsonpath='{range .items[*]}{range .status.conditions[?(@.type=="Ready")]}{.status}{"\n"}{end}{end}' 2>/dev/null | awk '$1 == "True" {count++} END {print count+0}')"
		if [[ "${ready}" == "2" ]]; then
			return 0
		fi
		sleep 1
	done
	echo "WarmLease WorkerPool did not reach two Ready workers: ${ready}" >&2
	kubectl -n kember-warm-e2e get workerpool,pods -o wide >&2 || true
	return 1
}

wait_warm_status_ready() {
	local status=""
	for _ in $(seq 1 120); do
		status="$(kubectl -n kember-warm-e2e get workerpool echo-warm -o jsonpath='{.status.capacity.desired} {.status.capacity.ready} {.status.capacity.leased} {.status.conditions[?(@.type=="Ready")].status} {.status.conditions[?(@.type=="Progressing")].status} {.status.conditions[?(@.type=="Degraded")].status}' 2>/dev/null || true)"
		if [[ "${status}" == "2 2 0 True False False" ]]; then
			return 0
		fi
		sleep 1
	done
	echo "WarmLease status did not converge: ${status}" >&2
	kubectl -n kember-warm-e2e get workerpool echo-warm -o yaml >&2 || true
	return 1
}

cleanup() {
	if [[ -n "${PORT_FORWARD_PID:-}" ]]; then
		kill "${PORT_FORWARD_PID}" 2>/dev/null || true
	fi
	kubectl delete namespace kember-e2e kember-warm-e2e --ignore-not-found --wait=true >/dev/null 2>&1 || true
}
trap cleanup EXIT

kind get clusters | grep -Fxq "${CLUSTER_NAME}"
mkdir -p "${CACHE_DIR}"
GOARCH="$(docker version --format '{{.Server.Arch}}')"
GOWORK=off GOCACHE="${PWD}/.cache/go-build" GOOS=linux GOARCH="${GOARCH}" CGO_ENABLED=0 \
	go build -o "${CACHE_DIR}/kember-operator" ./apps/kember-operator
docker build -t "${OPERATOR_IMAGE}" -f deploy/operator/Dockerfile "${CACHE_DIR}"
docker pull "${WORKER_IMAGE}"
kind load docker-image --name "${CLUSTER_NAME}" "${OPERATOR_IMAGE}"
kind load docker-image --name "${CLUSTER_NAME}" "${WORKER_IMAGE}"

KEMBER_OPERATOR_IMAGE="${OPERATOR_IMAGE}" ./deploy/install.sh
kubectl wait --for=condition=Established crd/workerpools.kember.dev crd/taskruns.kember.dev --timeout=60s

kubectl apply -f deploy/samples/e2e-success.yaml
wait_task_phase kember-e2e echo Succeeded

kubectl apply -l kember.dev/e2e-stage=pool -f deploy/samples/e2e-warm-single-use.yaml
wait_warm_ready
wait_warm_status_ready
kubectl apply -l kember.dev/e2e-stage=task -f deploy/samples/e2e-warm-single-use.yaml
wait_task_phase kember-warm-e2e echo-warm Succeeded

used_worker="$(kubectl -n kember-warm-e2e get taskrun echo-warm -o jsonpath='{.status.workerRef.name}')"
[[ -n "${used_worker}" ]]
kubectl -n kember-warm-e2e wait --for=delete "pod/${used_worker}" --timeout=60s
wait_warm_ready
wait_warm_status_ready

kubectl -n kember-system port-forward deployment/kember-operator "${METRICS_PORT}:8080" >/tmp/kember-alpha-metrics.log 2>&1 &
PORT_FORWARD_PID=$!
for _ in $(seq 1 30); do
	if curl --silent --fail "http://127.0.0.1:${METRICS_PORT}/metrics" >/tmp/kember-alpha-metrics.txt; then
		break
	fi
	sleep 1
done
[[ -s /tmp/kember-alpha-metrics.txt ]]
for metric in \
	kember_workerpool_ready_workers \
	kember_workerpool_leased_workers \
	kember_taskrun_active_duration_seconds \
	kember_taskrun_total \
	kember_worker_termination_requests_total \
	kember_taskrun_assignment_wait_seconds; do
	grep -q "^${metric}" /tmp/kember-alpha-metrics.txt
done

kubectl -n kember-e2e get taskrun echo -o wide
kubectl -n kember-warm-e2e get workerpool/echo-warm taskrun/echo-warm -o wide
echo "alpha install smoke passed"
