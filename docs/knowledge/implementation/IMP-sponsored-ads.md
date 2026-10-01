---
id: IMP-sponsored-ads
layer: implementation
title: 付费广告与投放实现映射
status: active
owner: agent
code_paths:
- app/gateway
- app/feed
- app/media
- app/behavior
- app/recommend/mq
- pkg/event
- deploy/sql
updated_at: 2026-10-01
---

# 付费广告与投放实现映射

付费广告与投放尚无实现。`code_paths` 暂列将被修改的既有入口；ad-rpc、ad-mq 与 `xbh_ad` schema 落地后
补充对应路径。`DISC-053` 与 `REL-009` 由广告投放实现承接，因此归属本页。全部条款在取得当前提交上的
有效证据前保持 `unknown`，计划周次见 [DES-sponsored-ads](../design/DES-sponsored-ads.md) 的分期表。

| requirement | design | state | evidence or gap |
| --- | --- | --- | --- |
| ADS-001 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W1，资质材料随 W2 补齐）。 |
| ADS-002 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W2）。 |
| ADS-010 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W1）。 |
| ADS-011 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W1）。 |
| ADS-012 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W1）。 |
| ADS-013 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W1）。 |
| ADS-014 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W1，申诉随 W6 补齐）。 |
| ADS-015 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W2）。 |
| ADS-016 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W2）。 |
| ADS-017 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W2）。 |
| ADS-020 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-021 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-022 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-023 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-024 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-025 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-026 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-027 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-030 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W6）。 |
| ADS-031 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W6）。 |
| ADS-032 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-040 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W1）。 |
| ADS-041 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-050 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-A01 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-A02 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W1～W2）。 |
| ADS-A03 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-A04 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-A05 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| ADS-A06 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W2）。 |
| ADS-A07 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W6）。 |
| ADS-A08 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W2）。 |
| DISC-053 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |
| REL-009 | DES-sponsored-ads | unknown | gap: 尚未实现（计划 W4）。 |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出，路径均相对仓库根目录。新服务落地时同步更新本页与
`docs/knowledge/guides/architecture.md`。
