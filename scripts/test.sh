#!/usr/bin/env bash
# Run `go test` across every module in the go.work workspace.
#
# With multiple go.mod files, `go test ./...` from the repo root only covers the
# root module, so iterate each module directory explicitly. Extra args (e.g.
# -run, -count) are forwarded to every invocation.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mapfile -t MODULES < <(
  find . -name go.mod -not -path './.worktrees/*' -not -path './vendor/*' \
    -exec dirname {} \; | sort
)

fail=0
for module in "${MODULES[@]}"; do
  echo "==> go test ${module}/..."
  if ! (cd "$module" && go test -race -cover "$@" ./...); then
    fail=1
  fi
done

exit "$fail"
