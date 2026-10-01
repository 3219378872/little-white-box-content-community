---
id: PROP-20261001-post-serving-supersede
layer: proposal
title: 投后任务（质检、举报、回扫）的作废时机：澄清 RVW-003
status: open
owner: agent
target_layer: spec
upstream:
  - SPEC-review-platform
  - SPEC-sponsored-ads
---

# 观察到的问题

`RVW-003` 规定「对象产生新 revision 后，旧 revision 的未决任务立即作废」。这条规则针对的是
「审核某个 revision 能否投放」的任务。广告是先审后投，并且 `ADS-011` 要求新版本审核期间继续投放
最近一次过审快照；质检（`RVW-014`）、举报（`ADS-030`）与回扫（`ADS-031`）三类投后任务审的正是这份
仍在投放的旧快照。

若按字面执行，广告主只要提交一次编辑：

- 回扫判定违规而暂停、正在等待人审的广告，人审任务被作废，广告一直停在暂停状态；新版本被拒时更是
  永远无法恢复或下线；
- 用户对在投快照的举报任务被作废，举报静默丢失；新版本审核期间到达的举报一送审就被判为旧 revision
  而直接作废；
- 自动通过后的抽样质检被作废，漏放率估计出现偏差。

# 建议变更

把 `RVW-003` 澄清为：

> 对象产生新 revision 后，旧 revision 的未决送审与申诉任务立即作废。针对在投过审快照的投后任务
> （质检、举报、回扫）在更新的 revision 过审、旧快照不再投放时作废。对作废任务的领取或提交返回
> 可区分的「任务已作废」结果。已作出的旧结论只保留审计，不再改变对象状态。

# 当前实现（W6，2026-10-01）

W6 已按上述澄清实现，以免投后处置在编辑场景下失效，并在
[DES-review-platform](../design/DES-review-platform.md) 与 [DES-sponsored-ads](../design/DES-sponsored-ads.md)
记为实现调整；`RVW-003` 在 [IMP-review-platform](../implementation/IMP-review-platform.md) 中以 gap 注明
与字面规则的差异。规格语义仍以人类决定为准。

# 需要人类决定

1. 是否接受上述澄清并修订 `SPEC-review-platform` 的 `RVW-003`。
2. 若不接受，投后任务在编辑后应如何处理（例如编辑时把已暂停的广告直接下线，或禁止对暂停中的广告编辑），
   实现需要随之调整。
