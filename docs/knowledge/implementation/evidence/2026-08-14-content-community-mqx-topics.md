---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 843d2a9
commands:
  - go build ./...
  - go test -count=1 ./deploy/... ./pkg/mqx/...
  - make check
  - make test
result: passed
---

# 2026-08-14 MQ 主题死代码清理与一致性护栏

## 本批次改动

- `pkg/mqx/topics.go`：删除 13 个零引用的主题常量（`TopicUserRegister`、
  `TopicUserFollow`/`Unfollow`、`TopicCommentCreate`/`Delete`、`TopicLike`/
  `Unlike`/`Favorite`/`Unfavorite`、`TopicSearchIndex`/`Delete`、
  `TopicUserBehavior`（旧）、`TopicFeedGenerate`）与 8 个零引用的消费者组
  常量（`GroupUserService` 等）。保留实际使用的 6 个主题
  （`post-create/update/delete`、`user-behavior-v2`、`message-push`、
  `media-deleted`）、`TagDefault` 与 `GroupBehaviorLogService`（行为日志管道
  消费者组，被 `app/pipeline/behaviorlog/internal/svc/service_context_test.go`
  引用；全量测试复查发现后保留）。
- `deploy/rocketmq_topics_test.go`：新增
  `TestRocketMQTopicsMatchCodeConstants`——断言 `topics.go` 常量集合与
  `init-topics.sh` 的 `TOPICS=(...)` 集合完全一致，防止代码与部署清单再次漂移。

## 审查证据

- 全仓非测试代码引用统计：被删的 15 个主题常量与 9 个组常量均为 0 引用；
  测试与 `docs/` 也无引用。
- 消费者订阅/发布实际使用的主题：`post-create`（search/feed/embedding 消费者 +
  content 发布）、`post-update`/`post-delete`（content 发布、cleanup 订阅）、
  `user-behavior-v2`（behavior/interaction/user/content 发布 + count-sync 订阅）、
  `message-push`（message 发布/订阅）、`media-deleted`（media 发布/订阅）。
- 部署脚本一致性：`init-topics.sh` 的 5 主题/8 消费者组与各服务 `etc/*.yaml`
  的 `GroupName`/`Topic` 逐一核对一致。

## 结果

- `go build ./...` 通过（删除后无遗漏引用）。
- `go test ./deploy/... ./pkg/mqx/...` 通过（含新一致性测试）。
- `make check` 通过；`make test` 全部模块通过（含 race）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
