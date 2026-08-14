---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: b4bd776
commands:
  - python3 -m unittest scripts.test_spec_evals
  - make check
  - make test
result: passed
---

# 2026-08-14 来源有效率计算语义修正（ASST-012/051，规格层验证）

## 规格偏离

`evaluate_assistant` 的来源有效率按 **recall 语义**计算：
`|期望 ∩ 返回| / |期望|`——模型返回伪造/无关来源（不在期望中）**不扣分**，
来源有效率仍可能 100%。违反 ASST-012（只有服务端验证过的来源可返回）
与 ASST-051（来源有效率必须 100%）——伪造引用不会被评测惩罚，
ASST-A03 无法被验证。

## 修复

`evaluate_assistant`：来源有效率改为 **precision 语义**：
`|期望 ∩ 返回| / |返回|`——返回的每个来源都必须有效（在期望中），
伪造/多余来源降低有效率。

## 测试

- 更新 `test_99_percent_source_accuracy_fails_gate`：构造 99/100 有效
  （1 个伪造）→ accuracy=0.99 → 门禁失败。
- 新增 `test_fabricated_source_is_penalized`：返回 [0, 999]（999 伪造）
  → accuracy=0.5 → 门禁失败（ASST-A03）。

## 结果

- `test_spec_evals` 31 项全过；`make check` 通过；`make test` 全部模块
  通过（含 race）。

## 未覆盖边界

- live 来源有效率（77.3%）按新语义重新计算后数值可能变化——live
  gate 结论（未达标）不变；外部输入门禁不变，见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
