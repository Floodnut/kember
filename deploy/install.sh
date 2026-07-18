#!/usr/bin/env bash
set -euo pipefail

# Install the alpha control plane into the current Kubernetes context.
# The image must already be available to the cluster (for example via
# `kind load docker-image`).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPERATOR_IMAGE="${KEMBER_OPERATOR_IMAGE:-kember-operator:e2e}"

kubectl apply -f "${SCRIPT_DIR}/operator/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/crd/kember.openflood.org_workerpools.yaml"
kubectl apply -f "${SCRIPT_DIR}/crd/kember.openflood.org_taskruns.yaml"
kubectl apply -f "${SCRIPT_DIR}/rbac/kember-operator.yaml"
kubectl apply -f "${SCRIPT_DIR}/operator/operator.yaml"
kubectl -n kember-system set image deployment/kember-operator "operator=${OPERATOR_IMAGE}"
kubectl -n kember-system rollout status deployment/kember-operator --timeout=120s

