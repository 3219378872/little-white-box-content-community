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

# 单模块仓库：goctl 生成不再需要临时 workspace 解析跨模块依赖。
user_proto_link="$ROOT_DIR/app/user/rpc/user.proto"
cleanup_generation() {
  if [[ -L "$user_proto_link" ]]; then
    unlink "$user_proto_link"
  fi
}
trap cleanup_generation EXIT

if [[ -e "$user_proto_link" || -L "$user_proto_link" ]]; then
  echo "refusing to replace existing $user_proto_link" >&2
  exit 1
fi
ln -s "$ROOT_DIR/proto/user/user.proto" "$user_proto_link"
(
  cd "$ROOT_DIR/app/user/rpc"
  goctl rpc protoc user.proto \
    --go_out=pb --go-grpc_out=pb --zrpc_out=. --style=go_zero
)
unlink "$user_proto_link"
goctl rpc protoc proto/message/message.proto \
  --go_out=app/message/rpc --go-grpc_out=app/message/rpc \
  --zrpc_out=app/message/rpc --style=go_zero
goctl rpc protoc proto/feed/feed.proto \
  --go_out=app/feed/rpc --go-grpc_out=app/feed/rpc \
  --zrpc_out=app/feed/rpc --style=go_zero
goctl rpc protoc proto/content/content.proto \
  --go_out=app/content/rpc/pb --go-grpc_out=app/content/rpc/pb \
  --zrpc_out=app/content/rpc --style=go_zero
goctl rpc protoc proto/interaction/interaction.proto \
  --go_out=app/interaction/rpc/pb --go-grpc_out=app/interaction/rpc/pb \
  --zrpc_out=app/interaction/rpc --style=go_zero
goctl rpc protoc proto/media/media.proto \
  --go_out=app/media/rpc/pb --go-grpc_out=app/media/rpc/pb \
  --zrpc_out=app/media/rpc --style=go_zero

protoc -I . --go_out=app/embedding/mq --go-grpc_out=app/embedding/mq \
  proto/embedding/embedding.proto
protoc -I . --go_out=app/recommend/rpc --go-grpc_out=app/recommend/rpc \
  proto/inference/inference.proto

app/embedding/service/generate_proto.sh
algorithm/online_infer/generate_proto.sh

goctl api go \
  --api "$ROOT_DIR/app/gateway/gateway.api" \
  --dir "$ROOT_DIR/app/gateway" \
  --style=go_zero --type-group

# gateway.api 声明 middleware: OptionalAuth，goctl 会在 internal/middleware 生成
# 空的 OptionalAuthMiddleware 桩（含误导性 TODO）；真实实现在 pkg/middleware，
# 路由经 serverCtx.OptionalAuth 装配。删除该死桩，保持生成后工作树干净。
rm -f "$ROOT_DIR/app/gateway/internal/middleware/optionalauth_middleware.go"
