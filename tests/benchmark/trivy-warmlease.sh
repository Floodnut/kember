#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

export WORKLOAD_NAME="trivy"
export WORKLOAD_IMAGE_TAG="${TRIVY_TAG:-aquasec/trivy:0.58.2}"
export WORKLOAD_IMAGE_REPOSITORY="aquasec/trivy"
export WORKLOAD_INPUT_REF="bench://fixture/secret.env"
export WORKLOAD_ALLOWED_PREFIX="bench://fixture/"
export WORKLOAD_COMMAND="rm -rf /tmp/kember-fixture && mkdir -p /tmp/kember-fixture && printf 'AWS_SECRET_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE\\n' > /tmp/kember-fixture/secret.env && exec trivy fs --scanners secret --skip-db-update --quiet --exit-code 0 /tmp/kember-fixture"
export WORKLOAD_READINESS_COMMAND="command -v trivy >/dev/null"
export NAMESPACE="${NAMESPACE:-kember-trivy-benchmark}"

exec "${ROOT}/tests/benchmark/checkov-warmlease.sh"
