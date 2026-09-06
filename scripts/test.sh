#!/usr/bin/env bash
# Run `go test` from every go.mod directory.
#
# The repository is a single module today; iterating go.mod roots keeps the
# gate correct if a nested module is added. Extra args are forwarded.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

source "$ROOT_DIR/scripts/_lib.sh"

mapfile -t MODULES < <(list_modules)

fail=0
for module in "${MODULES[@]}"; do
  echo "==> go test ${module}/..."
  module_name="$(printf '%s' "$module" | sed 's#^\./##; s#[/.]#_#g')"
  [[ -n "$module_name" ]] || module_name="root"
  if [[ -n "${TEST_JSON_DIR:-}" ]]; then
    mkdir -p "$TEST_JSON_DIR"
    if ! (cd "$module" && go test -json -race -cover "$@" ./... | tee "$TEST_JSON_DIR/$module_name.json"); then
      fail=1
    fi
  elif ! (cd "$module" && go test -race -cover "$@" ./...); then
    fail=1
  fi
done

exit "$fail"
