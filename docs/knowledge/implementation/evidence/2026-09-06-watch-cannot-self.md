---
implementation: IMP-content-community-backend
verified_at: 2026-09-06
verified_commit: 027ed32e589c4328c91664bb7176145ecdcb9335
commands:
  - go test -race -count=1 ./pkg/errx/ ./app/assistant/watch/ ./app/assistant/rpc/internal/logic/ ./app/assistant/internal/tool/
result: passed
---

# 2026-09-06 Watch 不能盯自己

## 范围

`CannotWatchSelf`（6005，「不能关注自己的动态」）拒绝 `author_new_post` 盯当前用户、
`post_revised` 盯当前用户自己的帖。`discussion_spike` 仍允许盯自己的帖。REST 与
`create_watch_task` 共用 `watch.Lookups.Validate`。

## 命令与结果

```text
go test -race -count=1 ./pkg/errx/ ./app/assistant/watch/ ./app/assistant/rpc/internal/logic/ ./app/assistant/internal/tool/
```

退出码 0。含 `TestLookupsRejectsWatchingOwnAuthorAndPost`、
`TestCreateWatchTaskLogicValidatesTargets` 盯自己失败路径，以及 `HTTPStatus`/`GRPCCode`
对 6005 的映射。

## 未证明范围

未在本证据中跑全量 `make test` / `make check`；根仓真实栈与 e2e 由编排仓记录。
