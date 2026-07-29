#!/usr/bin/env bash
set -euo pipefail

SERVICE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SERVICE_DIR/../.." && pwd)"
GENERATED_DIR="$SERVICE_DIR/generated"

mkdir -p "$GENERATED_DIR"
python3 -m grpc_tools.protoc \
  -I "$REPO_DIR/proto/inference" \
  --python_out="$GENERATED_DIR" \
  --grpc_python_out="$GENERATED_DIR" \
  "$REPO_DIR/proto/inference/inference.proto"
