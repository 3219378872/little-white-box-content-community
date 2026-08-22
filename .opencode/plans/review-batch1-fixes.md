# 第一批审查修复方案（已批准）

审查报告结论：M5 撤销（goctl 1.10.1 handler 带 `Safe to edit` 标记，再生成不覆盖）。
M2 加重：feed 的 `Config.Validate()` 已定义但全仓无调用点，需接线。

## 工作流

- worktree：`.worktree/review-batch1`，分支 `task/review-batch1-fixes`（已创建）
- 每项修复独立 commit；完成后 rebase main、推送、清理 worktree 和分支

## 修复项

### M2 配置校验接线
- `app/gateway/internal/config/config.go`：新增 `Validate()`，校验 `Auth.AccessSecret` 非空
  （TrimSpace，模式对齐 feed CursorSecret 校验）
- `app/gateway/gateway.go:30`：`conf.MustLoad` 后调用 `c.Validate()`，失败 fatal
- `app/user/rpc/internal/config/config.go`：新增 `Validate()`，校验 `JwtConfig.AccessSecret` 非空
- `app/user/rpc/user.go:29`：接线调用
- `app/feed/rpc/feed.go:24`：接线调用既有 `c.Validate()`
- 测试：gateway 与 user config 各补 Validate 单测（空 secret 失败路径）

### L3 MaxBytes 对齐
- `app/gateway/etc/gateway.yaml` RestConf 增加 `MaxBytes: 10485760`（对齐 upload handler 10MB）

### L2 手机注册密码强度
- `app/user/rpc/internal/logic/register_logic.go:76` `registerByPhone`：
  仅当 `req.Password != ""` 时调 `validator.CheckPasswordStrength`
  （空密码走 newUser 随机生成路径，不得拦截）
- 测试：register_logic_test 补弱密码拒绝失败路径

### L1 评论 ReplyUserId 归属校验
- `app/content/rpc/internal/logic/create_comment_logic.go:102`：
  当 `in.ReplyUserId > 0` 时加载父评论（ParentId），校验：父评论存在、同帖、作者一致，
  否则返回 `errx.ParamError`
- 测试：create_comment 测试补 mismatch 拒绝路径

### I1 favorites 路由 OptionalAuth
- `app/gateway/gateway.api` GetUserFavorites 块加 `middleware: OptionalAuth`
- goctl 再生成，diff 审查仅 routes.go 变化
- 更新 `rest_decision_table_test.go` 契约
- Logic 已就绪（GetUserIdFromContext + GetOptionalUserIdFromContext），无需改动

## 验证

- `make check`
- 目标模块 race 测试：user rpc、content rpc、gateway
- `.api` 再生成后 diff 审查

## 知识链合规

五项均为对齐既有安全约定（CORE/REL 编号控制项）的缺陷修复，不新增语义；
完成后在实现台账（IMP-content-community-backend.md）补记条目。
