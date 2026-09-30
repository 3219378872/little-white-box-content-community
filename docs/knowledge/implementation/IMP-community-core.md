---
id: IMP-community-core
layer: implementation
title: 社区核心实现映射
status: active
owner: agent
updated_at: 2026-09-30
code_paths:
- pkg/rpcx
- pkg/httpx
- pkg/configx
- pkg/logging
- pkg/sqlstore
- pkg/redisstore
- pkg/cachedstore
- kitex_gen
- pkg/interceptor
- app/content
- app/interaction
- app/message
- app/media
- app/user
- app/gateway
- pkg/idempotencyx
- pkg/outboxx
- pkg/visibilityx
- proto/content/content.proto
- proto/interaction/interaction.proto
- proto/message/message.proto
- proto/media/media.proto
- proto/user/user.proto
---

# 社区核心实现映射

内容生命周期、互动、私信、媒体、鉴权与可靠写入。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| requirement | design | state | evidence or gap |
| --- | --- | --- | --- |
| CORE-001 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-002 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-003 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-004 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-005 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-010 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-011 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-012 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-013 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-014 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-015 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-016 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-020 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-021 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-022 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-023 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-024 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-030 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-031 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-032 | DES-content-community-backend | unknown | gap: 访问者互动状态已可立即读取；公开计数 30 秒收敛仍缺生产观测。 |
| CORE-033 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-034 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-040 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-041 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-042 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-043 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-044 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-050 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-051 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-052 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-053 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-054 | DES-content-community-backend | unknown | gap: 媒体提交补偿路径已变更，原 CloudWeGo 证据输入过期；待新提交上的完整错误码与传输验收。 Assistant 接受输入改为事务内当前读，既有覆盖已过期；待本提交上的跨服务错误契约验收。 Memory/Watch 业务边界已改变，既有全量覆盖过期；待本提交上的跨服务错误契约验收。 缓存填充围栏与媒体权威读取路径已变更，旧错误响应证据不再覆盖当前实现；待最终提交上的对应验收证据。 HTTP 摄取连接读取截止时间的行为已改变；旧全量覆盖过期，待最终提交上的跨服务错误与隐私验收。 向量投影修改使既有大范围验收输入失效；本批 targeted race 已通过，完整框架兼容性门禁需重验。 |
| CORE-060 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-061 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-062 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-063 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-A01 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-A02 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-A03 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-A04 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-A05 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-A06 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| CORE-A07 | DES-content-community-backend | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录，
跨模块行为通过上游 DES 与本表中的精确条款关联。

## 证据边界

当前确定性验证见 `EVD-20260925-quality-remediation`。覆盖稳定账户登录锁、Lua 固定窗口、既存无 TTL 修复及全部社区回归；CORE-032 的公开计数收敛仍保持原有 unknown。
证据在固定实现提交运行全模块 race、静态检查与专用 MySQL/Redis 隔离集成，随后更新本映射；
历史 EVD 保留原观察结果，不改写为当前证明。原有 unknown/diverged 及其 gap 不提升。
本轮没有真实模型、浏览器、设备、容量或生产验证，这些范围不能从本地门禁推导。
