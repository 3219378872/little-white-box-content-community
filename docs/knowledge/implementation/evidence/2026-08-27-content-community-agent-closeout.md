---
implementation: IMP-content-community-backend
verified_at: 2026-08-27
verified_commit: 163bf1cbb71e2dda91cd35ee8801614b7b1e2052
commands:
  - goctl rpc protoc proto/interaction/interaction.proto --go_out=app/interaction/rpc/pb --go-grpc_out=app/interaction/rpc/pb --zrpc_out=app/interaction/rpc --style=go_zero
  - go test -count=1 ./app/assistant/... ./app/interaction/rpc/internal/logic/... ./app/gateway/
  - python3 scripts/engineering-lint.py
result: partial
---

# 2026-08-27 Agent Runtime 代码可关闭项收口

## 环境

- 工作树：`.worktree/task-agent-runtime-closeout`，分支 `task/agent-runtime-closeout`。
- 范围：UserState 工具、Watch 创建校验/命中回源/未读注入/`discussion_spike` 预筛选、
  推荐排除与卡片、审计落库、记忆卫生与列表 503。
- 未发明人类冻结评测集或月度 SLO 数据。

## 命令与结果

```text
goctl rpc protoc proto/interaction/interaction.proto \
  --go_out=app/interaction/rpc/pb --go-grpc_out=app/interaction/rpc/pb \
  --zrpc_out=app/interaction/rpc --style=go_zero
```

生成成功；未覆盖 interaction 主程序与 ServiceContext 定制。新增
`GetLikeList` 与 `FindActiveTargetIds`。

```text
go test -count=1 ./app/assistant/... ./app/interaction/rpc/internal/logic/... ./app/gateway/
```

全部 `ok`。

```text
python3 scripts/engineering-lint.py
```

`engineering-lint: all checks passed`。

## 映射结论

- UserState 四工具注册并回源已发布帖；v1 consent 仍隐藏。Interaction 新增
  `GetLikeList`。
- Watch 创建校验作者/已发布帖/标签；命中列表回源后对不可见条目清空标题摘要；
  Runtime 注入最多 5 条未读命中；matcher 订阅 `user-behavior-v2`，未达评论阈值
  不调 Judge、不写 hit。
- 推荐读取开放 Task 排除 ID，成功回源后发 `recommend` 卡片；Emit nil 不发。
- Runtime 异步可同步等待地写入 `agent_run`/`tool_call`（参数哈希，无用户正文）。
- 关闭个性化后 Ignore `behavior`；Extract/Apply 拒绝手机号/验证码/私信样值；
  列表无 store 返回 503。

## 未覆盖边界

- `discussion_spike` 生产未接 LLM：过阈值记 `failed`，不改写成规则命中。
- `ASST-050`/`ASST-051`/`DISC-060` 人类冻结集未重跑。
- `REL-030`～`043` 缺真实月度观测。
- 未跑 `make test` 全量 / `make integration-all`。
