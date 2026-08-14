#!/usr/bin/env bash
# Run `go test` across every module in the go.work workspace.
#
# With multiple go.mod files, `go test ./...` from the repo root only covers the
# root module, so iterate each module directory explicitly. Extra args (e.g.
# -run, -count) are forwarded to every invocation.
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
