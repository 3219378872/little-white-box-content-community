#!/usr/bin/env bash
set -euo pipefail

SERVICE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SERVICE_DIR/../../.." && pwd)"

python3 -m grpc_tools.protoc \
  -I "$REPO_DIR/proto/embedding" \
  --python_out="$SERVICE_DIR" \
  --grpc_python_out="$SERVICE_DIR" \
  "$REPO_DIR/proto/embedding/embedding.proto"
