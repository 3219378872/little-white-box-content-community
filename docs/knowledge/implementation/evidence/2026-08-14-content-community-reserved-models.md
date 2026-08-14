---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 4eb52e3
commands:
  - go build ./...
  - go test -count=1 ./app/interaction/...
  - make check
  - make test
result: passed
---

# 2026-08-14 interaction 预留 Model 清理

## 本批次改动

- 删除 `app/interaction/rpc/internal/model` 中三个从未接入的预留 Model
  （手写 + goctl 生成各一份）：
  - `favorite_folder_model.go` / `_gen.go`（收藏夹）
  - `report_model.go` / `_gen.go`（举报）
  - `view_history_model.go` / `_gen.go`（浏览历史）
- `service_context.go`：同步删除 `FavoriteFolderModel`/`ReportModel`/
  `ViewHistoryModel` 字段与构造。

## 审查证据

- 三个 Model 来自最早脚手架提交（21d1755「整理项目结构」），全仓无
  RPC 方法、无 gateway 路由、无测试引用；`proto/interaction` 无相关
  `rpc`；`app/gateway/gateway.api` 无相关路由；意图/规范/设计均未提及
  举报/浏览历史/收藏夹。
- 无交叉引用：`favorite_model.go`/`like_record_model.go`/
  `interaction_command_model.go` 不引用被删类型；`vars.go` 的
  `StatusInactive/StatusActive` 与缓存 TTL 常量仍被在用 Model 使用，保留。

## 观察项（本轮不改）

- `deploy/sql/xbh_interaction.sql` 中的 `favorite_folder`、`view_history`、
  `report` 表**保留**：删除生产 schema 属于迁移决策，不在本轮范围；
  表结构无运行时成本，未来如启用对应功能需设计评审后重建 Model。

## 结果

- `go build ./...` 通过；`go test ./app/interaction/...` 通过；
  `make check` 通过；`make test` 全部模块通过（含 race）。
- 生成同步不受影响（被删 Model 无 `.api`/`.proto` 上游）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
