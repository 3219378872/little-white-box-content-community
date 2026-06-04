# Fix Service Startup Issues

## Context

全量运行 7 个微服务后，发现 2 个服务启动失败、若干警告。本 spec 仅修复启动阻塞项，不涉及功能变更。

## Changes

### 1. Message RPC: 添加 `conf.UseEnv()` (CRITICAL)

- **文件**: `app/message/rpc/message.go:30`
- **变更**: `conf.MustLoad(*configFile, &c)` → `conf.MustLoad(*configFile, &c, conf.UseEnv())`
- **原因**: 缺少 env var 替换导致 `${MQ_NAMESERVER}` 字面量传入 RocketMQ 客户端，解析失败 fatal

### 2. Feed RPC: 端口 9091 → 9093 (CRITICAL)

- **文件**: `app/feed/rpc/etc/feed.yaml`
- **变更**: `ListenOn: 0.0.0.0:9091` → `ListenOn: 0.0.0.0:9093`
- **原因**: 9091 被 Milvus 容器占用

### 3. 补充 `DB_MESSAGE` 环境变量 (HIGH)

- **文件**: `scripts/env.sh`
- **变更**: 新增 `export DB_MESSAGE='root:Xbh@MySQL2024!@tcp(127.0.0.1:3306)/xbh_message?parseTime=true&loc=UTC'`

### 4. Content RPC: 添加日志级别 (LOW)

- **文件**: `app/content/rpc/etc/content.yaml`
- **变更**: 新增 `Log.Level: info`
- **原因**: 消除 "invalid log level" warning

## Non-Goals

- 不修改 docker-compose、Milvus 端口
- 不修改 Gateway RPC 客户端集成（后续迭代）
- 不修改 RocketMQ broker 网络配置（暂观察）
- 不修改 proto、不重新生成代码

## Verification

- `go build ./...` 通过
- 全部 7 个服务启动无 fatal/panic
- `go test ./... -race` 通过（不影响已有测试）
