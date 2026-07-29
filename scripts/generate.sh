#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

for tool in goctl protoc protoc-gen-go protoc-gen-go-grpc python3; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "missing code generation tool: $tool" >&2
    exit 1
  fi
done

if ! python3 -c 'import grpc_tools.protoc' >/dev/null 2>&1; then
  echo "missing Python code generation module: grpc_tools.protoc" >&2
  echo "install it with: python3 -m pip install --requirement app/embedding/service/requirements-dev.txt" >&2
  exit 1
fi

goctl rpc protoc proto/behavior/behavior.proto \
  --go_out=app/behavior/rpc --go-grpc_out=app/behavior/rpc \
  --zrpc_out=app/behavior/rpc --style=go_zero
goctl rpc protoc proto/search/search.proto \
  --go_out=app/search/rpc --go-grpc_out=app/search/rpc \
  --zrpc_out=app/search/rpc --style=go_zero
goctl rpc protoc proto/recommend/recommend.proto \
  --go_out=app/recommend/rpc --go-grpc_out=app/recommend/rpc \
  --zrpc_out=app/recommend/rpc --style=go_zero
goctl rpc protoc proto/assistant/assistant.proto \
  --go_out=app/assistant/rpc --go-grpc_out=app/assistant/rpc \
  --zrpc_out=app/assistant/rpc --style=go_zero

generation_work_dir="$(mktemp -d)"
user_work_dir="$generation_work_dir/user"
gateway_work_dir="$generation_work_dir/gateway"
user_proto_link="$ROOT_DIR/app/user/rpc/user.proto"
cleanup_generation() {
  if [[ -L "$user_proto_link" ]]; then
    unlink "$user_proto_link"
  fi
  rm -rf "$generation_work_dir"
}
trap cleanup_generation EXIT

mkdir -p "$user_work_dir"
(
  cd "$user_work_dir"
  go work init "$ROOT_DIR/app/user/rpc"
)
if [[ -e "$user_proto_link" || -L "$user_proto_link" ]]; then
  echo "refusing to replace existing $user_proto_link" >&2
  exit 1
fi
ln -s "$ROOT_DIR/proto/user/user.proto" "$user_proto_link"
(
  cd "$ROOT_DIR/app/user/rpc"
  GOWORK="$user_work_dir/go.work" goctl rpc protoc user.proto \
    --go_out=pb --go-grpc_out=pb --zrpc_out=. --style=go_zero
)
unlink "$user_proto_link"
goctl rpc protoc proto/message/message.proto \
  --go_out=app/message/rpc --go-grpc_out=app/message/rpc \
  --zrpc_out=app/message/rpc --style=go_zero
goctl rpc protoc proto/feed/feed.proto \
  --go_out=app/feed/rpc --go-grpc_out=app/feed/rpc \
  --zrpc_out=app/feed/rpc --style=go_zero

protoc -I . --go_out=app/embedding/mq --go-grpc_out=app/embedding/mq \
  proto/embedding/embedding.proto
protoc -I . --go_out=app/recommend/rpc --go-grpc_out=app/recommend/rpc \
  proto/inference/inference.proto

app/embedding/service/generate_proto.sh
algorithm/online_infer/generate_proto.sh

mkdir -p "$gateway_work_dir"
(
  cd "$gateway_work_dir"
  go work init "$ROOT_DIR/app/gateway"
)
GOWORK="$gateway_work_dir/go.work" goctl api go \
  --api "$ROOT_DIR/app/gateway/gateway.api" \
  --dir "$ROOT_DIR/app/gateway" \
  --style=go_zero --type-group
