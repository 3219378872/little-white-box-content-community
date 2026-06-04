# GetPostList Optional Auth Design

## Goal

将 `GET /api/v1/posts` 明确定义为公开内容流入口，允许未登录用户访问，同时支持在携带合法 JWT 时将用户信息注入 `context`，为后续个性化字段扩展提供基础，但不阻塞匿名访问。

## Background

当前网关中：

- `GET /api/v1/posts` 位于 `posts` 的 `jwt: Auth` 分组下
- `GetPostList` gateway logic 并未消费当前用户信息
- `GetUserPosts` 已被实现为公开接口
- `GetUserFavorites` 已采用“公开路由 + logic 内按 requester/visibility 判断”的模式

这导致 `GetPostList` 的路由鉴权边界和业务语义不一致。对于公开内容流，要求登录会直接损伤匿名访问体验，也不利于 SEO、分享落地页、冷启动浏览和未登录转化。

## Product Decision

### Endpoint Role

`GET /api/v1/posts` 是公开内容流，不是仅登录后可见的首页流。

### Access Policy

- 未登录用户可以直接访问
- 已登录用户可以访问
- 请求携带合法 JWT 时，网关注入用户信息
- 请求未携带 JWT 时，按匿名请求处理
- 请求携带过期、非法、格式错误的 JWT 时，公开内容仍可访问，不返回 `401`

### UX Principle

公开内容不能因为登录态异常而不可达，但登录态异常也不能对前端“无感”，否则会形成隐形降级：用户以为自己仍在登录状态，实际服务端已经按匿名处理。

## Options Considered

### Option A: Keep `GET /posts` in required-JWT group

优点：

- 不需要新中间件
- 强登录态接口行为简单

缺点：

- 与“公开内容流”定位冲突
- 匿名用户被 `401` 阻断
- 当前 logic 不依赖用户态，强鉴权没有收益

### Option B: Split anonymous feed into a second endpoint

例如保留当前 `GET /posts` 为登录接口，再新增匿名 feed 接口。

优点：

- 可以最小化改动已有登录态路径

缺点：

- 语义重复
- 前端和文档复杂度上升
- 缓存、埋点、排序策略容易分叉

### Option C: Make `GET /posts` public and support optional JWT

优点：

- 与产品定位一致
- 匿名访问和登录访问共用一个内容入口
- 为未来个性化增强保留扩展点

缺点：

- 需要新增可选 JWT 中间件
- 需要统一公开接口上的 token 失效信号

### Recommendation

选择 Option C。

## Design

### 1. Route Boundary

将 `GET /api/v1/posts` 从强制 JWT 的 `posts` 分组移出，放入公开读分组。

强制 JWT 的边界保持不变：

- `POST /api/v1/post`
- `PUT /api/v1/post/:postId`
- `DELETE /api/v1/post/:postId`
- 其他写操作接口

本次设计不要求同步调整 `GetPost`、`GetCommentList`，但中间件和 helper 的设计应允许它们未来沿用同一模式。

### 2. Optional JWT Middleware

新增一个专用于“公开但可利用登录态增强”的 HTTP 中间件。该中间件不替代现有强制鉴权中间件，两者职责如下：

- `AuthMiddleware`: 强制登录，没有合法 token 就返回 `401`
- `OptionalAuthMiddleware`: 尽力解析 token，失败时不阻断请求

`OptionalAuthMiddleware` 行为定义：

1. 请求没有 `Authorization` 头  
   直接放行，不注入用户信息

2. 请求带有合法 Bearer token  
   解析 claims，向 `context` 注入用户信息后放行

3. 请求带有过期、非法、格式错误 token  
   不返回 `401`，不注入用户信息，按匿名请求放行

### 3. Context Contract

公开接口不能再假设上下文里一定有用户信息。后续 logic 必须遵守以下约定：

- 有用户信息时，可以读取当前用户 ID
- 无用户信息时，必须能安全走匿名分支
- “取不到用户”不是系统错误

建议统一通过 helper 读取，而不是在各处直接 `ctx.Value(...)` 做脆弱类型断言。

推荐 helper 语义：

- `GetOptionalUserIdFromContext(ctx) (int64, bool)`  
  返回 `userId` 和是否存在

或等价 helper，只要能明确区分“匿名请求”和“读取失败”即可。

### 4. GetPostList Logic Contract

`GetPostList` 在当前版本返回公共列表数据，不依赖登录态。

当前阶段要求：

- 匿名请求返回公共帖子列表
- 合法登录请求返回同一公共帖子列表
- token 异常请求也返回公共帖子列表

未来如果要补充 `IsLiked`、`IsFavorited` 等个性化字段，应遵循兼容扩展原则：

- 有用户态时返回真实个性化值
- 无用户态或 token 无效时返回匿名默认值
- 不因为个性化不可用而阻断公共列表访问

### 5. Auth State Signaling

为解决“公开接口在 token 失效时前端无感知，导致用户体验隐形降级”的问题，公开接口在可选鉴权场景下需要返回非阻塞鉴权状态信号。

推荐通过响应头实现，而不是污染响应 body。

建议响应头：

- `X-Auth-State: authenticated`
- `X-Auth-State: anonymous`
- `X-Auth-State: expired`
- `X-Auth-State: invalid`

定义如下：

- `authenticated`: 请求携带合法 token，且已注入上下文
- `anonymous`: 请求未携带 token
- `expired`: 请求携带过期 token
- `invalid`: 请求携带格式错误、签名错误或不可解析 token

### 6. Frontend Behavior Contract

前端需要同时承担“主动感知”和“被动纠偏”两层责任。

#### 6.1 Active Detection

客户端本地持有 JWT 时，应在请求前检查 `exp`：

- 已过期 token 不继续携带
- 本地登录态立即切换为失效
- 给用户轻量提示或拉起登录入口

#### 6.2 Server Signal Handling

前端网络层统一读取 `X-Auth-State`：

- `authenticated`: 维持当前登录态
- `anonymous`: 正常匿名处理
- `expired` / `invalid`: 清理本地 token，切换全局用户态为未登录，并向用户显示轻提示

这样既不会阻断用户浏览公开内容，也不会让用户长时间误以为自己仍处于登录状态。

### 7. Error Handling

强制鉴权接口保持现状：

- 无 token / 非法 token / 过期 token -> `401 Unauthorized`

公开可选鉴权接口采用降级访问：

- 无 token -> `200`
- 合法 token -> `200`
- 过期 token -> `200` + `X-Auth-State: expired`
- 非法 token -> `200` + `X-Auth-State: invalid`

服务端应记录结构化日志，但不把 token 内容写入日志。

### 8. Scope

本设计聚焦以下变更：

- 明确 `GetPostList` 为公开接口
- 引入可选 JWT 中间件
- 统一公开接口的上下文读取契约
- 增加公开接口上的鉴权状态信号

本设计不包含：

- Interaction 服务接入后的个性化字段实现
- 评论列表、帖子详情等其他读接口的同步迁移
- 前端完整登录流改造

## Affected Areas

预期会影响的代码区域：

- `app/gateway/gateway.api`
- `app/gateway/internal/handler/routes.go`
- `pkg/middleware/auth.go` 或相邻中间件文件
- `pkg/jwtx/jwt.go` 或相邻 helper 文件
- `app/gateway/internal/logic/posts/get_post_list_logic.go`
- 与公开读接口相关的测试文件

## Verification

### Middleware Coverage

至少覆盖以下场景：

- 无 `Authorization` 头 -> 放行，`X-Auth-State: anonymous`
- 合法 Bearer token -> 放行，注入用户信息，`X-Auth-State: authenticated`
- 过期 token -> 放行，不注入用户信息，`X-Auth-State: expired`
- 非法 token -> 放行，不注入用户信息，`X-Auth-State: invalid`

### Endpoint Coverage

至少覆盖以下场景：

- `GET /api/v1/posts` 无 token -> `200`
- `GET /api/v1/posts` 合法 token -> `200`
- `GET /api/v1/posts` 过期 token -> `200`
- `GET /api/v1/posts` 非法 token -> `200`
- `POST /api/v1/post` 无 token -> `401`

### Logic Contract Coverage

至少覆盖：

- 匿名上下文调用 `GetPostList`
- 有用户上下文调用 `GetPostList`

即使当前返回结果相同，也要通过测试固定“公开接口不依赖强登录态”这一契约。

## Risks

### Risk 1: Public endpoint silently hides auth failures

如果没有响应头信号，前端可能长期持有失效 token，导致用户误判登录状态。

缓解：

- 增加 `X-Auth-State`
- 前端网络层统一消费

### Risk 2: Existing context reader is brittle

当前 `GetUserIdFromContext` 假设值类型为 `json.Number`，与中间件写入方式并不稳固。

缓解：

- 为公开接口引入更明确的 optional helper
- 避免把“未读到用户”视为系统错误

### Risk 3: Optional auth semantics drift across endpoints

如果后续各接口各自实现“半公开半鉴权”，行为会发散。

缓解：

- 将可选 JWT 中间件做成统一设施
- 固定 `X-Auth-State` 枚举
- 在后续公开读接口迁移时复用同一模式

## Success Criteria

- `GET /api/v1/posts` 在无 token 时可正常访问
- 带坏 token 不阻断公开内容访问
- 前端能通过统一信号感知登录态失效
- 强制鉴权接口仍保持 `401` 语义
- 后续为公开读接口引入个性化增强时，无需再次推翻路由和鉴权模型
