#!/usr/bin/env bash
set -euo pipefail

exec "${TEST_SRCDIR}/${TEST_WORKSPACE}/packages/worker-contract/proto/kember/plugin/v1/plugin_kotlin_compatibility"
