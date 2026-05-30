#!/usr/bin/env bash
# Run `go vet` across every module in the go.work workspace.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mapfile -t MODULES < <(
  find . -name go.mod -not -path './.worktrees/*' -not -path './vendor/*' \
    -exec dirname {} \; | sort
)

fail=0
for module in "${MODULES[@]}"; do
  echo "==> go vet ${module}/..."
  if ! (cd "$module" && go vet ./...); then
    fail=1
  fi
done

exit "$fail"
