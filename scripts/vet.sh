#!/usr/bin/env bash
# Run `go vet` from every go.mod directory. The repository is a single module
# today; the loop stays so a nested module cannot silently skip vet.
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
