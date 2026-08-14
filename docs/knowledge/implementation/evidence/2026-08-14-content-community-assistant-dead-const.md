---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: ac45c2c
commands:
  - go build ./...
  - go test -count=1 ./app/assistant/...
  - make check
result: passed
---

# 2026-08-14 assistant 工具名死常量清理

## 本批次改动

- `app/assistant/rpc/internal/tool/registry.go`：删除 `Name.User`（`"user"`）
  常量。`NewRegistry` 只注册 `search`/`content`/`recommend` 三个工具，
  `user` 从未实现且全仓（含测试）零引用；`AllowedTools` 配置也不包含
  `user`。

## 审查证据（本轮复查）

- 全仓导出常量/类型零引用扫描：手写 app 包中除 `Name.User` 外均为同文件
  或同包引用（`PostIndexMapping`、`NotificationType*`、`WireAPI*` 均误报
  排除后复核确认在用）；`New*Logic` 构造函数全部有调用方。
- 正向验证：outboxx relay（默认值/校验/退避/租约）、recommend rank
  （规则权重/推理降级/NaN 防护）、assistant registry（白名单/fail-closed/
  证据构造）、Dockerfile 多阶段非 root 构建、production compose 服务齐全，
  均无质量问题。

## 结果

- `go build ./...` 通过；`go test ./app/assistant/...` 通过；`make check`
  通过。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
