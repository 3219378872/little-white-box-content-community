# 测试与交付速查

## 最低要求

- 每个 Logic 至少覆盖一个失败路径。
- 纯 SQL 断言可以使用 sqlmock；涉及真实 DB/Redis/RPC 的集成行为使用仓库已有 testcontainers 工具。
- 不 mock `sqlx.SqlConn` 来伪造集成测试。

## 验证命令

```bash
python3 scripts/engineering-lint.py
scripts/test.sh
scripts/vet.sh
scripts/lint.sh
```

根据变更范围执行相关命令；报告实际执行结果。生成文件、文档链接、知识库同步和代码质量由 `engineering-lint.py` 与 CI 检查。
