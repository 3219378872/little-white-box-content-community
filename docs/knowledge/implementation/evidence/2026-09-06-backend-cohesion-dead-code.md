---
implementation: IMP-content-community-backend
verified_at: 2026-09-06
verified_commit: c4db7e6
commands:
  - make check
  - go test -race -count=1 ./pkg/errx/ ./pkg/util/ ./pkg/middleware/ ./app/user/rpc/internal/password/ ./app/user/rpc/internal/logic/ ./app/assistant/internal/tool/ ./app/content/rpc/internal/logic/ ./app/feed/rpc/internal/logic/ ./app/message/rpc/internal/logic/
  - make test
result: passed
---

# 2026-09-06 低耦合高内聚：死代码清理与包边界收敛

## 缺陷 / 清理

运行时从未接线的 goctl Model：

- message 的 verify_code model：message 库误生成的验证码表 Model，ServiceContext 未持有。
- media 的 media_task model：媒体任务表无 RPC/消费者。
- content 的 category model：仅集成测试读写脚手架，无产品入口。
- user 的 user_login_log model：ServiceContext 持有但 Logic 从不写入或读取。

其它死面：

- middleware `GetUserId` / `GetUsername`：全仓仅兼容测试引用，业务走 `jwtx`。
- `errx.AlreadyLiked` / `AlreadyFavorited` / `NotLikedYet` / `NotFavoritedYet` /
  `CannotLikeSelf`：赞藏是 CORE-030 幂等成功，Logic 从不返回这些码。
- feed 的 DTM 残留编译测试：DTM 移除后的空类型断言。

## 内聚重构

- 密码哈希从 `pkg/util` 迁到 `app/user/rpc/internal/password`（user 专属）。
- JSON 列编解码从 `pkg/util` 迁到 `app/content/rpc/internal/model`（content 专属）。
- `pkg/util` 只保留 Snowflake。
- Assistant 工具执行器由单文件拆为 `exec_{search,content,recommend,social,memory,watch,web,sources,helpers}.go`，同包无行为变化。

SQL 基线表保留（与 2026-08-14 reserved models 同一边界）：删除生产 schema 需独立
迁移决策。通知 RPC / `notificationUnread` 仍被 Gateway 未读摘要使用，不删。

## 文档

重写 `IMP-architecture` 共享库与包边界；刷新 `IMP-engineering-conventions` 错误码
与 JWT 读取约定；`scripts/{lint,test,vet,_lib}.sh` 不再假装多 module workspace。
