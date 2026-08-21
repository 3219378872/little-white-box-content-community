#!/usr/bin/env bash
# Run `go vet` across every module in the repository.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

source "$ROOT_DIR/scripts/_lib.sh"

mapfile -t MODULES < <(list_modules)

fail=0
for module in "${MODULES[@]}"; do
  echo "==> go vet ${module}/..."
  if ! (cd "$module" && go vet ./...); then
    fail=1
  fi
done

exit "$fail"
