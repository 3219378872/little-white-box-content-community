---
id: PROP-20260813-slo-synthetic-observation
layer: proposal
title: 用 LLM 生成合成生产观测进行 SLO 报告管线干跑
status: open
owner: agent
target_layer: spec
upstream:
  - SPEC-feedback-reliability
---

# 观察到的缺口

- `REL-030~043` / `REL-A05` 要求以 UTC 自然月窗口与统一口径生成可用性、延迟、异步延迟、
  RPO/RTO 报告，输入为真实生产观测数据。
- 当前没有生产环境观测数据，`IMP-todo-blocked-gates` 将该门禁登记为待外部输入，
  报告管线（`scripts/spec_evals.py slo`）无法被端到端验证。

# 建议

人类已于 2026-08-14 授权用 LLM 生成“测试用生产数据”。落地方式：
- `eval/slo/profiles.json`：由 LLM（deepseek-v4-flash，opencodego 账户）生成的
  6 个能力域月度观测画像（请求量、lognormal 延迟参数、不可用率、排除率、日期窗口）。
- `scripts/gen_slo_synthetic.py`：从画像确定性合成请求数组（固定种子，可复现），
  产出 `eval/slo/2026-07-<capability>.json`。
- 干跑结果：6 个能力域 `spec_evals.py slo` 全部 `met=True`，证明报告管线
  （REL-030/031 分母口径、p95 计算、阈值判断）正确。

# 需要人类决定的事项

1. 是否接受合成观测作为 REL-030~033/040~043 的**门禁关闭依据**？
   - 推荐：**否**。合成数据验证的是报告管线，不代表生产合规；
     REL 行保持 `partial`，待真实生产数据替换后按 REL-A05 正式出报告。
   - 若接受（可选）：需在 SPEC-feedback-reliability 增加“合成观测干跑”条款并批准。

# 影响

- 数据：`eval/slo/*.json` 合成观测（明确标记 synthetic）。
- 文档：IMP-todo、IMP REL 行、证据页。
- 不改动：`spec_evals.py slo` 口径（合成数据必须通过同一管线）。
