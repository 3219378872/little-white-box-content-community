#!/usr/bin/env bash
# Run bounded native fuzz targets used by the nightly workflow.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fuzz_time="${FUZZ_TIME:-20s}"
go test -run='^$' -fuzz='^FuzzValidatorsNeverPanic$' -fuzztime="$fuzz_time" ./pkg/validator
go test -run='^$' -fuzz='^FuzzBehaviorEventJSONNeverPanics$' -fuzztime="$fuzz_time" ./pkg/event
(cd pkg/jwtx && go test -run='^$' -fuzz='^FuzzParseTokenNeverPanics$' -fuzztime="$fuzz_time" .)
go test -run='^$' -fuzz='^FuzzDetectNeverPanics$' -fuzztime="$fuzz_time" ./app/media/rpc/internal/mediautil
