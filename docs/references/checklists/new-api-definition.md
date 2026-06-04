# .api 文件修改检查清单

> 适用场景：修改 `.api` 文件 → goctl 重新生成代码

---

## 🚨 阻塞型检查

### API 定义规范

- [ ] 语法通过 `goctl api validate --api xxx.api`
- [ ] 所有 type 有明确的 JSON tag
- [ ] 所有 handler 有完整的 `info(` 注释（summary、desc）
- [ ] Path 参数、Query 参数、Body 参数正确使用对应 tag
  - `path:"id"` / `form:"page"` / `json:"name"`
- [ ] HTTP 动词语义正确（GET 不改状态、POST 创建、PUT 全量更新、PATCH 部分更新、DELETE 删除）

### 版本与兼容性

- [ ] 路由前缀包含版本号（`/api/v1/`）
- [ ] **禁止**修改已发布接口的路径（新版本用新路径）
- [ ] **禁止**删除已发布接口的响应字段（标记 deprecated，下个大版本删）
- [ ] **禁止**修改已发布字段的 JSON key 或类型
- [ ] 新增字段必须是可选的（零值不破坏现有客户端）

### 参数与校验

- [ ] 必填字段使用 `validate:"required"`
- [ ] 字符串字段有长度限制（`validate:"max=255"`）
- [ ] 枚举字段使用 `validate:"oneof=a b c"`
- [ ] 分页字段有合理上限（`page_size <= 100`）
- [ ] ID 字段类型统一（全项目要么 int64 要么 string，不混用）

### 响应格式

- [ ] 遵循统一响应包装（`{code, msg, data}`）
- [ ] 列表响应包含分页元信息（`total`、`page`、`page_size`）
- [ ] 时间字段统一使用 ISO 8601 或 Unix 时间戳（不混用）
- [ ] 错误响应使用统一错误码表

### 代码生成后

- [ ] 已运行 `goctl api go -api xxx.api -dir . --style go_zero`
- [ ] `internal/types/types.go` 已更新
- [ ] `internal/handler/routes.go` 已更新
- [ ] `internal/handler/*handler.go` 已为新接口生成空壳
- [ ] `internal/logic/*logic.go` 已生成对应方法
- [ ] **没有**遗留手动编辑的 goctl 生成代码（会被覆盖）

### 文档同步

- [ ] Swagger / OpenAPI 文档已更新（如使用 goctl swagger）
- [ ] 前端 SDK / 客户端代码已同步生成
- [ ] 变更日志 CHANGELOG 已记录

---

## ⚠️ 建议型检查

### RESTful 最佳实践

- [ ] 资源命名使用**复数名词**（`/users` 而非 `/user`）
- [ ] 嵌套资源深度 ≤ 2（`/users/:id/posts` 可以，`/users/:id/posts/:pid/comments` 考虑拆分）
- [ ] 动作用 HTTP 动词表达而非路径（避免 `/getUser`）
- [ ] 查询过滤使用 query 参数（`?status=active&sort=-created_at`）

### 安全

- [ ] 敏感接口在 `@server(middleware: JwtAuth)` 标注
- [ ] 管理员接口在 `@server(middleware: JwtAuth,AdminOnly)` 标注
- [ ] 公开接口明确标注 `@server(middleware: -)` 或单独 group
