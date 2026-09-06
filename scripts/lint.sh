#!/usr/bin/env bash
# Run golangci-lint from every go.mod directory.
#
# The repository is a single module today; the loop stays so a nested module
# or extra go.mod cannot silently skip lint. Extra args are forwarded.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

source "$ROOT_DIR/scripts/_lib.sh"

if ! command -v golangci-lint >/dev/null 2>&1; then
  cat >&2 <<'MSG'
golangci-lint is not installed or not on PATH.

The prebuilt binaries are built with an older Go and reject this repo's
go 1.27.0 config, so build it from source with the project toolchain:
  GOTOOLCHAIN=go1.27.0 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1

Then rerun:
  scripts/lint.sh
MSG
  exit 127
fi

mapfile -t MODULES < <(list_modules)

fail=0
for module in "${MODULES[@]}"; do
  echo "==> golangci-lint run ${module}/..."
  if ! (cd "$module" && golangci-lint run "$@" ./...); then
    fail=1
  fi
done

exit "$fail"
