#!/usr/bin/env bash
set -euo pipefail

ROLE_FILE="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/deploy/rbac/kember-operator.yaml"

if grep -Eq '\["\*"\]' "${ROLE_FILE}"; then
	echo "operator RBAC must not contain wildcard apiGroups, resources, or verbs" >&2
	exit 1
fi

for required in \
	'apiGroups: ["kember.openflood.org"]' \
	'apiGroups: ["batch"]' \
	'apiGroups: [""]' \
	'apiGroups: ["coordination.k8s.io"]' \
	'resources: ["pods/exec"]' \
	'verbs: ["create"]'; do
	grep -Fq "${required}" "${ROLE_FILE}"
done

echo "operator RBAC contract passed"
