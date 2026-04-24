#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v golangci-lint >/dev/null 2>&1; then
  cat >&2 <<'MSG'
golangci-lint is not installed or not on PATH.

Install it from the official docs:
  https://golangci-lint.run/welcome/install/

Then rerun:
  scripts/lint.sh
MSG
  exit 127
fi

golangci-lint run "$@" ./...
