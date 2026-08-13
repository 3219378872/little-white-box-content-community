---
id: IMP-content-community-backend
layer: implementation
title: 小白盒内容社区后端实现映射
status: diverged
owner: agent
upstream:
  - DES-content-community-backend
tracks:
  - app/content/rpc/internal/logic
  - app/content/rpc/internal/model
  - app/interaction/rpc/internal/logic
  - app/user/rpc/internal/logic
  - app/message/rpc/internal/logic
  - app/media/rpc/internal/logic
  - app/recommend/rpc/internal/logic
  - app/recommend/mq/internal/store
  - app/assistant/rpc/internal/logic
  - app/assistant/rpc/internal/store
  - app/assistant/rpc/internal/tool
  - app/behavior/rpc/internal/logic
  - app/search/rpc/internal/logic
  - app/gateway
  - proto/content/content.proto
  - proto/message/message.proto
  - proto/media/media.proto
  - proto/user/user.proto
  - proto/assistant/assistant.proto
  - proto/behavior/behavior.proto
  - proto/search/search.proto
  - deploy/sql/xbh_content.sql
  - deploy/sql/xbh_user.sql
  - deploy/sql/xbh_media.sql
  - deploy/sql/xbh_analytics.sql
verified_at: 2026-08-13
verified_commit: f089c0d
---

# 小白盒内容社区后端实现映射

本页记录四份已批准规范到代码实现的映射。状态与证据以
[DES-content-community-backend](../design/DES-content-community-backend.md)
的逐条追踪为准；此处只登记入口与总体对齐结论。

## 总体状态

`diverged`：本次已按规格修复匿名公开读取、搜索/推荐查询时回源、Assistant 无证据拒答、
以及个性化偏好读取失败 fail-closed。仍偏离处：
- `CORE-013` 与 `CORE-062` 冲突，v1 仍允许 `expectedRevision=0` 跳过乐观锁。
- `DISC-060~063` / `ASST-050~051` 冻结评测集待人类评审。
- `REL-030~043` 月度 SLO/异步延迟缺少生产观测。

## 代码入口

- 内容生命周期与幂等：`app/content/rpc/internal/logic/{create,update,delete,get}_post_logic.go`、
  `comment_logic`、`app/content/rpc/internal/model/{post,comment}_command_model.go`、
  `app/content/rpc/internal/model/idempotency_model.go`。
- 互动：`app/interaction/rpc/internal/logic/{like,unlike,favorite,unfavorite}_logic.go`。
- 用户与隐私：`app/user/rpc/internal/logic/{get,set}_personalization_preference_logic.go`、
  `app/user/rpc/internal/model/personalization_preference_model.go`。
- 私信：`app/message/rpc/internal/logic/send_message_logic.go`、
  `app/message/rpc/internal/model/message_command_model.go`。
- 媒体：`app/media/rpc/internal/logic/upload_{image,video}_logic.go`、
  `app/media/rpc/internal/model/media_command_model.go`、
  `app/media/rpc/internal/model/idempotency_model.go`。
- 推荐：`app/recommend/rpc/internal/logic/get_recommend_posts_logic.go`、`helpers.go`、
  `app/recommend/mq/internal/store/behavior_store.go`。
- Assistant：`app/assistant/rpc/internal/logic/chat_logic.go`、
  `app/assistant/rpc/internal/tool/registry.go`、`app/assistant/rpc/internal/store/state.go`。
- 行为：`app/behavior/rpc/internal/logic/record_events_logic.go`、`pkg/event/behavior.go`。
- 搜索：`app/search/rpc/internal/logic/search_logic.go`。
- 网关：`app/gateway/internal/logic/**` 与 `app/gateway/gateway.api`。

## 证据

验证于 2026-08-12（提交 0031d91）：
- `make check`：fmt、engineering-lint、vet、golangci-lint 全部通过。
- `make test`：全部模块 race 测试通过。
- `go test -tags integration ./app/content/rpc/internal/logic/` 与
  `./app/user/rpc/internal/logic/`：通过（含 revision/idempotency/状态机/隐私偏好）。
- `make integration-critical` 通过；count-sync 与 message 命令模型集成测试通过。
- 各服务单元测试覆盖新增失败路径（版本冲突、幂等冲突、媒体归属、来源变化等）。

未覆盖边界：媒体与消息的媒体校验依赖真实 media RPC（本地无 SeaweedFS），通过单元测试
fake 覆盖；离线评测门禁与 SLO 观测数据不在此提交内。
