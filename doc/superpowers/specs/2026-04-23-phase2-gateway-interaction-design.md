# Phase 2 Gateway Interaction 对接设计

## 日期
2026-04-23

## 背景
Phase 2 的 Interaction RPC 服务已生成并可用。Gateway 已有 Like/Unlike/Favorite/Unfavorite 的 API 定义和空 Logic，以及一个带 TODO 的 GetUserFavorites。本设计旨在将 Gateway 的现有互动接口实际接入 Interaction RPC。

## 范围
仅 Gateway 层改动，不涉及 Feed/Message 服务。

## 设计详情

### 1. 配置层

文件：`app/gateway/internal/config/config.go`

在 `Config` 结构体中新增：

```go
InteractionRpc zrpc.RpcClientConf
```

### 2. 依赖注入层

文件：`app/gateway/internal/svc/service_context.go`

在 `ServiceContext` 中新增：

```go
InteractionService interactionservice.InteractionService
```

初始化逻辑与现有 `UserService`/`ContentService`/`MediaService` 保持一致，使用相同的 `BizErrorUnaryInterceptor`。

### 3. Logic 层实现

#### 3.1 Like / Unlike

文件：`app/gateway/internal/logic/like_favorite/like_logic.go`、`unlike_logic.go`

- 从 JWT Context 获取 `userId`
- 调用 `InteractionService.Like` / `Unlike`
- RPC 请求字段：`user_id`, `target_id`, `target_type`
- 成功返回空响应

#### 3.2 Favorite / Unfavorite

文件：`app/gateway/internal/logic/like_favorite/favorite_logic.go`、`unfavorite_logic.go`

- 从 JWT Context 获取 `userId`
- 调用 `InteractionService.Favorite` / `Unfavorite`
- RPC 请求字段：`user_id`, `post_id`
- 成功返回空响应

#### 3.3 GetUserFavorites

文件：`app/gateway/internal/logic/user/get_user_favorites_logic.go`

数据流：
1. 权限检查（已有逻辑，保持不变）
2. 调用 `InteractionService.GetFavoriteList` 获取 `post_ids`
3. 调用 `ContentService.GetPostsByIds` 批量获取帖子详情
4. 组装为 `[]PostItem` 返回

```
Gateway → Interaction.GetFavoriteList → post_ids
      → Content.GetPostsByIds         → PostInfo 列表
      → 组装 PostItem 返回前端
```

### 4. 错误处理

- RPC 调用错误统一通过 `interceptor.BizErrorUnaryInterceptor` 转换
- Logic 层使用 `l.Errorw(...)` 记录失败日志，携带 context 信息
- 返回错误统一使用 `errx.NewWithCode(errx.SystemError)`

### 5. 测试策略

- 每个 Logic 至少覆盖：成功路径 + 失败路径
- `GetUserFavorites` 额外覆盖：权限通过但列表为空、权限通过有数据、RPC 失败
- 遵循项目 TDD 规范（RED-GREEN-REFACTOR）

## 交付物

| 文件 | 改动 |
|------|------|
| `app/gateway/internal/config/config.go` | 新增 `InteractionRpc` |
| `app/gateway/internal/svc/service_context.go` | 新增 `InteractionService` |
| `app/gateway/internal/logic/like_favorite/like_logic.go` | 实现调用 Interaction RPC |
| `app/gateway/internal/logic/like_favorite/unlike_logic.go` | 实现调用 Interaction RPC |
| `app/gateway/internal/logic/like_favorite/favorite_logic.go` | 实现调用 Interaction RPC |
| `app/gateway/internal/logic/like_favorite/unfavorite_logic.go` | 实现调用 Interaction RPC |
| `app/gateway/internal/logic/user/get_user_favorites_logic.go` | 接入 Interaction + Content RPC |
| 各 Logic 对应的 `_test.go` | 单元测试 |

## 风险

- `GetUserFavorites` 涉及两次 RPC 调用（Interaction + Content），需处理级联失败
- 若 `InteractionService` 未启动，Gateway 启动时会因 `zrpc.MustNewClient` 失败
