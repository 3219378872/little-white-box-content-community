#!/usr/bin/env bash
# Run the full integration suite in an isolated, disposable environment.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_SCRIPT="$ROOT_DIR/scripts/integration-env.sh"
TEST_RUNNER="${INTEGRATION_TEST_RUNNER:-$ROOT_DIR/scripts/integration-test.sh}"

finalize() {
  local status=$? clear_status=0
  trap - EXIT INT TERM
  if ((status != 0)); then
    "$ENV_SCRIPT" logs || true
  fi
  "$ENV_SCRIPT" clear || clear_status=$?
  if ((status == 0 && clear_status != 0)); then
    status=$clear_status
  fi
  exit "$status"
}

trap finalize EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"$ENV_SCRIPT" init
"$TEST_RUNNER" --all
"$ROOT_DIR/algorithm/integration/run.sh"
