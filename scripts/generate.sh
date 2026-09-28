#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

for tool in kitex protoc protoc-gen-go protoc-gen-go-grpc python3; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "missing code generation tool: $tool" >&2
    exit 1
  fi
done

python_codegen_requirements="$ROOT_DIR/scripts/requirements-generate.txt"
if ! python3 - "$python_codegen_requirements" <<'PY'
import importlib.metadata
import pathlib
import sys

import grpc_tools.protoc  # noqa: F401 - fail before generation if the module is broken

requirements = pathlib.Path(sys.argv[1])
pins = {}
for raw in requirements.read_text(encoding="utf-8").splitlines():
    line = raw.strip()
    if not line or line.startswith("#"):
        continue
    package, separator, version = line.partition("==")
    if not separator or not package or not version:
        raise SystemExit(f"invalid exact codegen pin: {line}")
    pins[package] = version

for package in pins:
    expected = pins.get(package)
    if expected is None:
        raise SystemExit(f"missing codegen pin: {package}")
    try:
        actual = importlib.metadata.version(package)
    except importlib.metadata.PackageNotFoundError:
        raise SystemExit(f"missing Python code generation package: {package}")
    if actual != expected:
        raise SystemExit(
            f"Python code generation package {package}={actual}; expected {expected}"
        )
PY
then
  echo "install pinned Python generators with:" >&2
  echo "  python3 -m pip install --requirement scripts/requirements-generate.txt" >&2
  exit 1
fi

# Pin the generator as tightly as the runtime: its private client layout is used
# by the generated lifecycle adapter.
[[ "$(kitex --version 2>&1)" == "v0.16.2" ]] || { echo 'kitex v0.16.2 required' >&2; exit 1; }
[[ "$(protoc-gen-go --version)" == "protoc-gen-go v1.36.12" ]] || { echo 'protoc-gen-go v1.36.12 required' >&2; exit 1; }
[[ "$(protoc-gen-go-grpc --version)" == "protoc-gen-go-grpc 1.6.1" ]] || { echo 'protoc-gen-go-grpc v1.6.1 required' >&2; exit 1; }
export KITEX_TOOL_USE_PROTOC=1
for rpc_proto in \
  proto/user/user.proto \
  proto/content/content.proto \
  proto/media/media.proto \
  proto/interaction/interaction.proto \
  proto/feed/feed.proto \
  proto/message/message.proto \
  proto/search/search.proto \
  proto/recommend/recommend.proto \
  proto/assistant/assistant.proto \
  proto/behavior/behavior.proto; do
  kitex -module esx -I . "$rpc_proto"
done
python3 scripts/generate_rpc_adapters.py
python3 scripts/generate_gateway.py

# The Python sidecars retain standard gRPC clients and wire contracts.
protoc -I . --go_out=app/embedding/mq --go-grpc_out=app/embedding/mq proto/embedding/embedding.proto
protoc -I . --go_out=app/recommend/rpc --go-grpc_out=app/recommend/rpc proto/inference/inference.proto
app/embedding/service/generate_proto.sh
algorithm/online_infer/generate_proto.sh
