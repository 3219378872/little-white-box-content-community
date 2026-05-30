#!/usr/bin/env bash
# Run golangci-lint across every module in the go.work workspace.
#
# This repo is split across multiple nested Go modules, so a single root-level
# `golangci-lint run ./...` only covers the root module. Iterate each module
# directory (mirrors scripts/test.sh and scripts/vet.sh). golangci-lint walks up
# to find the shared root .golangci.yml from each module. Extra args are
# forwarded to every invocation.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v golangci-lint >/dev/null 2>&1; then
  cat >&2 <<'MSG'
golangci-lint is not installed or not on PATH.

The prebuilt binaries are built with an older Go and reject this repo's
go 1.26.1 config, so build it from source with the project toolchain:
  GOTOOLCHAIN=go1.26.1 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.0

Then rerun:
  scripts/lint.sh
MSG
  exit 127
fi

mapfile -t MODULES < <(
  find . -name go.mod -not -path './.worktrees/*' -not -path './vendor/*' \
    -exec dirname {} \; | sort
)

fail=0
for module in "${MODULES[@]}"; do
  echo "==> golangci-lint run ${module}/..."
  if ! (cd "$module" && golangci-lint run "$@" ./...); then
    fail=1
  fi
done

exit "$fail"
