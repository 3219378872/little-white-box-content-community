#!/usr/bin/env bash
# Run govulncheck from every go.mod directory.
#
# The repository is a single module today; the loop stays so a nested module
# cannot silently skip the vulnerability scan. Extra args are forwarded.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

source "$ROOT_DIR/scripts/_lib.sh"

if ! command -v govulncheck >/dev/null 2>&1; then
  cat >&2 <<'MSG'
govulncheck is not installed or not on PATH.

Install it with the project toolchain:
  GOTOOLCHAIN=go1.27.0 go install golang.org/x/vuln/cmd/govulncheck@v1.7.0

Then rerun:
  scripts/govulncheck.sh
MSG
  exit 127
fi

mapfile -t MODULES < <(list_modules)

fail=0
for module in "${MODULES[@]}"; do
  echo "==> govulncheck ${module}/..."
  if ! (cd "$module" && govulncheck "$@" ./...); then
    fail=1
  fi
done

exit "$fail"
