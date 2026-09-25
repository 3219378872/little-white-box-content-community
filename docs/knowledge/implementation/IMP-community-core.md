---
id: IMP-community-core
layer: implementation
title: 社区核心实现映射
status: active
owner: agent
updated_at: 2026-09-25
code_paths:
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
| CORE-001 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-002 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-003 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-004 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-005 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-010 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-011 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-012 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-013 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-014 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-015 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-016 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-020 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-021 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-022 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-023 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-024 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-030 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-031 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-032 | DES-content-community-backend | unknown | gap: 访问者互动状态已可立即读取；公开计数 30 秒收敛仍缺生产观测。 |
| CORE-033 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-034 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-040 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-041 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-042 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-043 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-044 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-050 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-051 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-052 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-053 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-054 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-060 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-061 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-062 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-063 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-A01 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-A02 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-A03 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-A04 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-A05 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-A06 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| CORE-A07 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录，
跨模块行为通过上游 DES 与本表中的精确条款关联。

## 证据边界

当前确定性验证见 `EVD-20260925-quality-remediation`。覆盖稳定账户登录锁、Lua 固定窗口、既存无 TTL 修复及全部社区回归；CORE-032 的公开计数收敛仍保持原有 unknown。
证据在固定实现提交运行全模块 race、静态检查与专用 MySQL/Redis 隔离集成，随后更新本映射；
历史 EVD 保留原观察结果，不改写为当前证明。原有 unknown/diverged 及其 gap 不提升。
本轮没有真实模型、浏览器、设备、容量或生产验证，这些范围不能从本地门禁推导。
