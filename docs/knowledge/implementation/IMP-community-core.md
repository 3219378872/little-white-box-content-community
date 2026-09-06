---
id: IMP-community-core
layer: implementation
title: 社区核心实现映射
status: unknown
owner: agent
upstream:
  - DES-content-community-backend
updated_at: 2026-09-06
tracks:
  - CORE-001
  - CORE-002
  - CORE-003
  - CORE-004
  - CORE-005
  - CORE-010
  - CORE-011
  - CORE-012
  - CORE-013
  - CORE-014
  - CORE-015
  - CORE-016
  - CORE-020
  - CORE-021
  - CORE-022
  - CORE-023
  - CORE-024
  - CORE-030
  - CORE-031
  - CORE-032
  - CORE-033
  - CORE-034
  - CORE-040
  - CORE-041
  - CORE-042
  - CORE-043
  - CORE-044
  - CORE-050
  - CORE-051
  - CORE-052
  - CORE-053
  - CORE-054
  - CORE-060
  - CORE-061
  - CORE-062
  - CORE-063
  - CORE-A01
  - CORE-A02
  - CORE-A03
  - CORE-A04
  - CORE-A05
  - CORE-A06
  - CORE-A07
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
evidence:
  - EVD-20260906-community-core
---

# 社区核心实现映射

内容生命周期、互动、私信、媒体、鉴权与可靠写入。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| Requirement | Design | Status | Evidence/Gap |
| --- | --- | --- | --- |
| CORE-001 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-002 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-003 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-004 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-005 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-010 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-011 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-012 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-013 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-014 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-015 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-016 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-020 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-021 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-022 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-023 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-024 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-030 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-031 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-032 | DES-content-community-backend | unknown | gap: viewer state 已同步；公开计数 30 秒收敛仍缺生产观测。 |
| CORE-033 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-034 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-040 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-041 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-042 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-043 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-044 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-050 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-051 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-052 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-053 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-054 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-060 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-061 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-062 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-063 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-A01 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-A02 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-A03 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-A04 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-A05 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-A06 | DES-content-community-backend | unknown | gap: validation pending. |
| CORE-A07 | DES-content-community-backend | unknown | gap: validation pending. |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录，
跨模块行为通过上游 DES 与本表中的精确条款关联。

## 证据边界

当前确定性验证见 `EVD-20260906-community-core`。旧 `implementation/evidence/` 记录只保留历史上下文，
不参与当前 `aligned` 判定。人类、真实 provider、浏览器、设备和生产证据未执行时不会被推断。
