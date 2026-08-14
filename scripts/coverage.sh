#!/usr/bin/env bash
# Generate coverage profiles for every go.work module and enforce layered gates.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

source "$ROOT_DIR/scripts/_lib.sh"

gate="baseline"
if [[ "${1:-}" == "--target" ]]; then
  gate="target"
  shift
elif [[ "${1:-}" == "--no-gate" ]]; then
  gate="none"
  shift
fi

output_dir="$ROOT_DIR/.artifacts/coverage"
rm -rf "$output_dir"
mkdir -p "$output_dir/profiles"

mapfile -t modules < <(list_modules)

for module in "${modules[@]}"; do
  module_name="$(printf '%s' "$module" | sed 's#^\./##; s#[/.]#_#g')"
  [[ -n "$module_name" ]] || module_name="root"
  echo "==> coverage $module/..."
  (cd "$module" && go test -count=1 -covermode=atomic -coverpkg=./... \
    -coverprofile="$output_dir/profiles/$module_name.out" "$@" ./...)
done

python3 scripts/coverage_report.py "$output_dir/profiles" \
  --thresholds scripts/coverage-thresholds.json \
  --gate "$gate" \
  --output "$output_dir/report.json"

for profile in "$output_dir"/profiles/*.out; do
  name="$(basename "$profile" .out)"
  go tool cover -html="$profile" -o "$output_dir/$name.html"
done
