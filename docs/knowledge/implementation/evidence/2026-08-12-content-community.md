---
implementation: IMP-content-community-backend
verified_at: 2026-08-12
verified_commit: 0031d91
commands:
  - make check
  - make test
  - go test -tags integration -count=1 ./app/content/rpc/internal/logic/
  - go test -tags integration -count=1 ./app/user/rpc/internal/logic/
  - make integration-critical
  - go test -tags integration -count=1 ./app/content/mq/cleanup/internal/store/
  - go test -tags integration -count=1 ./app/message/rpc/internal/model/
result: passed
---

## 环境

- Go 1.26.1（GOTOOLCHAIN=go1.26.1）；testcontainers 启动 MySQL 8.0 + Redis 7。
- 分支 `main`，基线提交 48dd253。

## 结果

- `make check` 通过（fmt-check、engineering-lint、vet、golangci-lint 0 issues）。
- `make test` 全部模块通过（含 race）。
- content 集成测试通过：帖子创建/更新/删除、revision 冲突、幂等重试、draft⇄published、
  评论幂等。
- user 集成测试通过：注册/登录/验证码、个性化偏好 round-trip 与 Redis 标记。
- 互动/用户/内容 集成（make integration-critical）通过。
- count-sync 消费者 MySQL 集成测试通过（like/favorite 计数收敛与去重）。
- message 命令模型集成测试通过（media_id 持久化与幂等冲突）。

## 未覆盖边界

- media/message 的媒体校验依赖真实 media RPC，本地未起 SeaweedFS，仅单元 fake 覆盖。
- 推荐与行为链路的 Redis 集成测试需要真实 Redis + RocketMQ，未在本提交内执行。
- DISC-060~063 / ASST-050~051 离线评测集与脚本、REL SLO 观测数据未生成。
- 离线评测脚本已就绪（scripts/spec_evals.py），冻结数据集待人类双评标注。
