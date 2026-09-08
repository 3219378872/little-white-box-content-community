---
id: IMP-content-discovery
layer: implementation
title: 内容发现实现映射
status: active
owner: agent
updated_at: 2026-09-08
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
---

# 内容发现实现映射

关注流、搜索、推荐、可见性回源与离线质量门禁。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| requirement | design | state | evidence or gap |
| --- | --- | --- | --- |
| DISC-001 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-002 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-003 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-004 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-010 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-011 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-012 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-020 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-021 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-022 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-023 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-030 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-031 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-032 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-033 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-034 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-035 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-036 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-040 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-041 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-042 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-050 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-051 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-052 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-060 | DES-content-community-backend | unknown | gap: 当前数据为合成开发集；缺两名人类独立评审并消歧的正式 qrels。 |
| DISC-061 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-062 | DES-content-community-backend | diverged | gap: 默认 OnlineInfer / ModelVersion:auto 缺少 10k exposures + 1k identities 晋级门禁。 |
| DISC-063 | DES-content-community-backend | diverged | gap: 当前规则模型相对规则基线提升为 0，未达到 5% 与 bootstrap 下界要求。 |
| DISC-A01 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-A02 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-A03 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-A04 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-A05 | DES-content-community-backend | aligned | EVD-20260908-review-remediation |
| DISC-A06 | DES-content-community-backend | diverged | gap: 人类双评审搜索集缺失，且学习排序效果门禁已知未达到。 |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录，
跨模块行为通过上游 DES 与本表中的精确条款关联。

## 证据边界

当前确定性验证见 `EVD-20260908-review-remediation`，含普通/降级游标、负反馈失败关闭和真实 Redis
特征版本接线。旧 `implementation/evidence/` 记录只保留历史上下文，
不参与当前 `aligned` 判定。人类、真实 provider、浏览器、设备和生产证据未执行时不会被推断。
