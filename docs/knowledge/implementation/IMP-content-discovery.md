---
id: IMP-content-discovery
layer: implementation
title: 内容发现实现映射
status: diverged
owner: agent
upstream:
  - DES-content-community-backend
updated_at: 2026-09-06
tracks:
  - DISC-001
  - DISC-002
  - DISC-003
  - DISC-004
  - DISC-010
  - DISC-011
  - DISC-012
  - DISC-020
  - DISC-021
  - DISC-022
  - DISC-023
  - DISC-030
  - DISC-031
  - DISC-032
  - DISC-033
  - DISC-034
  - DISC-035
  - DISC-036
  - DISC-040
  - DISC-041
  - DISC-042
  - DISC-050
  - DISC-051
  - DISC-052
  - DISC-060
  - DISC-061
  - DISC-062
  - DISC-063
  - DISC-A01
  - DISC-A02
  - DISC-A03
  - DISC-A04
  - DISC-A05
  - DISC-A06
code_paths:
  - app/feed
  - app/search
  - app/recommend
  - app/embedding
  - algorithm
  - app/content/visibility
  - pkg/visibilityx
  - scripts/spec_evals.py
  - eval/dev/search_qrels.synthetic.json
  - eval/dev/recommend_samples.synthetic.json
evidence:
  - EVD-20260906-content-discovery
---

# 内容发现实现映射

关注流、搜索、推荐、可见性回源与离线质量门禁。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| Requirement | Design | Status | Evidence/Gap |
| --- | --- | --- | --- |
| DISC-001 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-002 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-003 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-004 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-010 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-011 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-012 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-020 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-021 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-022 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-023 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-030 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-031 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-032 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-033 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-034 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-035 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-036 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-040 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-041 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-042 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-050 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-051 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-052 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-060 | DES-content-community-backend | unknown | gap: 当前数据为合成开发集；缺两名人类独立评审并消歧的正式 qrels。 |
| DISC-061 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-062 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-063 | DES-content-community-backend | diverged | gap: 当前规则模型相对规则基线提升为 0，未达到 5% 与 bootstrap 下界要求。 |
| DISC-A01 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-A02 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-A03 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-A04 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-A05 | DES-content-community-backend | unknown | gap: validation pending. |
| DISC-A06 | DES-content-community-backend | diverged | gap: 人类双评审搜索集缺失，且学习排序效果门禁已知未达到。 |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录，
跨模块行为通过上游 DES 与本表中的精确条款关联。

## 证据边界

当前确定性验证见 `EVD-20260906-content-discovery`。旧 `implementation/evidence/` 记录只保留历史上下文，
不参与当前 `aligned` 判定。人类、真实 provider、浏览器、设备和生产证据未执行时不会被推断。
