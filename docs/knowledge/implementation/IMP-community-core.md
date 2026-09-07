---
id: IMP-community-core
layer: implementation
title: 社区核心实现映射
status: active
owner: agent
updated_at: 2026-09-06
code_paths:
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
---

# 社区核心实现映射

内容生命周期、互动、私信、媒体、鉴权与可靠写入。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| requirement | design | state | evidence or gap |
| --- | --- | --- | --- |
| CORE-001 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-002 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-003 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-004 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-005 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-010 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-011 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-012 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-013 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-014 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-015 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-016 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-020 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-021 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-022 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-023 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-024 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-030 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-031 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-032 | DES-content-community-backend | unknown | gap: viewer state 已同步；公开计数 30 秒收敛仍缺生产观测。 |
| CORE-033 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-034 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-040 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-041 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-042 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-043 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-044 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-050 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-051 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-052 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-053 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-054 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-060 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-061 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-062 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-063 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-A01 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-A02 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-A03 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-A04 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-A05 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-A06 | DES-content-community-backend | aligned | EVD-20260906-community-core |
| CORE-A07 | DES-content-community-backend | aligned | EVD-20260906-community-core |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录，
跨模块行为通过上游 DES 与本表中的精确条款关联。

## 证据边界

当前确定性验证见 `EVD-20260906-community-core`。旧 `implementation/evidence/` 记录只保留历史上下文，
不参与当前 `aligned` 判定。人类、真实 provider、浏览器、设备和生产证据未执行时不会被推断。
