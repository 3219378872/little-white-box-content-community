---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: ea0daa0
commands:
  - python3 scripts/gen_slo_synthetic.py
  - python3 scripts/spec_evals.py slo --requests eval/slo/2026-07-<capability>.json --capability <capability>
result: passed
---

# 2026-08-14 SLO 报告管线合成观测干跑验证（REL-030~033/040~043 管线）

## 背景

人类授权（2026-08-14）用 LLM 生成测试用生产数据：`eval/slo/profiles.json` 由
deepseek-v4-flash（opencodego 账户）生成 6 个能力域画像；`scripts/gen_slo_synthetic.py`
从画像确定性合成月度请求数组（固定种子 1，可复现）。

> 修正（2026-08-14 复查）：原实现用 Python 内置 `hash()` 做种子派生，
> 受 `PYTHONHASHSEED` 进程随机化影响，同一输入不同进程输出不同——
> "固定种子可复现"声明不成立。已改为 `sha256(capability)` 派生确定性
> 种子（SEED=1），两次生成完全一致、6 能力域仍全部 met=True。

## 干跑结果（scripts/spec_evals.py slo）

| 能力域 | 可用性 | 要求 | p95 | 要求 | met |
| --- | ---: | ---: | ---: | ---: | ---: |
| community_core_read | 0.99932 | ≥0.999 | 139.0ms | ≤300ms | True |
| community_core_write | 1.00000 | ≥0.999 | 387.3ms | ≤500ms | True |
| behavior_ingest | 0.99965 | ≥0.999 | 139.0ms | ≤300ms | True |
| discovery | 0.99616 | ≥0.995 | 686.3ms | ≤800ms | True |
| assistant_first_event | 0.99303 | ≥0.990 | 1535.2ms | ≤2000ms | True |
| assistant_completion | 0.99240 | ≥0.990 | 10247.3ms | ≤12000ms | True |

## 结论

- SLO 报告管线（REL-030/031 分母口径、p95 计算、阈值判断、降级/拒答计可用）端到端正确。
- 合成数据**不构成生产合规证据**：REL-030~043 保持 partial/n/a，待真实自然月生产观测
  替换后按 REL-A05 正式出报告（见 PROP-20260813-slo-synthetic-observation，待人类决定）。

## 其他门禁

- `make engineering-lint` 通过；`make check`/`make test`/`make coverage` 通过。
