---
implementation: IMP-content-community-backend
verified_at: 2026-08-27
verified_commit: a1942c466bc75d5b9265a50d4e4b21422dfa598f
commands:
  - go test -count=1 ./app/assistant/rpc/internal/agent/... ./app/assistant/rpc/internal/memory/... ./app/assistant/rpc/internal/watch/... ./app/assistant/rpc/internal/logic/... ./app/gateway/internal/logic/assistant/... ./app/gateway/
  - python3 -m unittest -v test_spec_evals.py
  - python3 scripts/engineering-lint.py
result: partial
---

# 2026-08-27 Agent Runtime 实现映射与评测回归

## 环境

- 工作树：`.worktree/task-agent-runtime-imp`，分支 `task/agent-runtime-imp`。
- 代码基线：`c33a541 feat(assistant): add condition watch tasks and rule matcher`。
- 本证据对应知识映射提交（`verified_commit` 在提交后回写为本提交 SHA）。
- 本机 Gateway `:8888` 未起来（`curl http://127.0.0.1:8888/health` 失败），live
  Gateway 评测跳过。
- `DB_ASSISTANT` 可选；本轮单测使用 MapStore，未连 `xbh_assistant`。

## 命令与结果

```text
go test -count=1 ./app/assistant/rpc/internal/agent/... \
  ./app/assistant/rpc/internal/memory/... \
  ./app/assistant/rpc/internal/watch/... \
  ./app/assistant/rpc/internal/logic/... \
  ./app/gateway/internal/logic/assistant/... \
  ./app/gateway/
```

全部 `ok`（agent 0.073s、memory 0.006s、watch 0.006s、logic 0.025s、
gateway assistant 0.022s、gateway 0.105s）。

```text
cd scripts && python3 -m unittest -v test_spec_evals.py
```

35 tests `OK`（Makefile `spec-evals-test` 入口）。未改
`eval/assistant_cases.json`，不宣称 ASST-050 关闭。

```text
python3 scripts/engineering-lint.py
```

映射文档写入后复跑；须通过。

## 映射结论

- Agent 模式、consent_version v1 五件套、记忆 CRUD/衰减/冲突、Watch `Match()`
  四种规则与未知条件拒绝：单测覆盖，对应条款可标 `aligned`。
- Watch RocketMQ 消费者进程未接线：`pkg/mqx.GroupAssistantWatchMatcher` 与
  `deploy/rocketmq/init-topics.sh` 预留 `assistant-watch-matcher-group`，仓库内没有
  assistant Watch matcher 消费者进程。WCH-010/013 及相关验收标 `partial`。
- 无 `DB_ASSISTANT` 时 REST 列表返回空，写接口 503。
- UserState 工具、`discussion_spike` 匹配、`agent_run` 落库、推荐卡片事件仍缺。

## 未覆盖边界

- 未重跑 live ASST-050/051（网关未启动，且 SPEC 要求人类冻结集）。
- 未跑 `make test` 全量 / `make integration-all`。
- Watch 命中未在下次对话注入；命中列表未回源可见性。
- 记忆未接个性化关闭（MEM-012），无私密字段丢弃测试（MEM-013）。
