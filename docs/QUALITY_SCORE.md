# 测试与代码质量标准

## 测试要求

- **最低覆盖率**：80%
- **必须包含**：单元测试 + 集成测试（涉及 DB / Redis / RPC）
- **每个 Logic** 至少一个失败路径测试

## 测试策略

| 类型 | 工具 | 用途 |
|------|------|------|
| SQL 断言 | sqlmock | 纯 SQL 逻辑验证 |
| 集成测试 | testcontainers | 真实 DB/Redis 端到端 |
| RPC mock | gomock | 跨服务调用隔离 |

- **禁止** mock `sqlx.SqlConn`
- **推荐** testcontainers 跑真实数据库

## 代码质量

- 函数 < 50 行
- 文件 < 800 行
- 嵌套 < 4 层
- 不硬编码配置值
- 不静默吞错误

## CI 门禁

```bash
go test ./... -race -cover
go vet ./...
golangci-lint run
```

## 按需参考

- [测试与交付速查](active/testing.md)
