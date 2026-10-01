---
id: IMP-sponsored-ads
layer: implementation
title: 付费广告与投放实现映射
status: active
owner: agent
code_paths:
- app/ad/internal
- app/ad/rpc
- app/ad/mq
- proto/ad/ad.proto
- app/gateway/internal/logic/ads
- app/gateway/internal/logic/feed
- app/recommend/mq/internal/mqs
- pkg/adpolicy
- deploy/sql/xbh_ad.sql
- deploy/sql/patches/20261001_ads_post_review.sql
- deploy/sql/xbh_analytics.sql
- deploy/seaweedfs
updated_at: 2026-10-01
---

# 付费广告与投放实现映射

付费广告 W1、W2、W4 与 W6 已于 2026-10-01 实现（广告主与资质、广告写路径与送审、结论应用、私有素材、投放
索引、频控与隐藏、推荐流合并、广告事件隔离与聚合；W6 为举报批次、申诉与政策回扫的暂停、下线与恢复），
前端联调属前端仓 W5。`DISC-053`
与 `REL-009` 由广告投放实现承接，因此归属本页。全部条款在取得当前提交上的有效 EVD 覆盖组前保持
`unknown`；gap 写明已实现内容、已执行测试与仍缺的验证。

| requirement | design | state | evidence or gap |
| --- | --- | --- | --- |
| ADS-001 | DES-sponsored-ads | unknown | gap: 已实现：广告主申请与资质随 revision 送审，未过审不能投放广告；集成测试 `TestAdvertiserLifecycle` 通过。尚无 EVD 覆盖组。 |
| ADS-002 | DES-sponsored-ads | unknown | gap: 已实现：受监管行业需目标市场有效资质，到期由 ad-mq 置为 expired 并移出投放；集成测试通过；站内通知尚未接入。尚无 EVD 覆盖组。 |
| ADS-010 | DES-sponsored-ads | unknown | gap: 已实现审核状态与读时计算的投放资格；当前创建即送审，`draft` 状态不可达。尚无 EVD 覆盖组。 |
| ADS-011 | DES-sponsored-ads | unknown | gap: 已实现：编辑产生新 revision 并送审，approved_revision 不变；集成测试 `TestEditApprovedAdKeepsServingOldSnapshot` 通过。尚无 EVD 覆盖组。 |
| ADS-012 | DES-sponsored-ads | unknown | gap: 已实现：投放内容只读取过审 revision 的 `ad_snapshot`，广告主名称取自过审主体名称。尚无 EVD 覆盖组。 |
| ADS-013 | DES-sponsored-ads | unknown | gap: 已实现：expectedRevision 冲突返回 2007，幂等键沿用 `pkg/idempotencyx`；集成测试通过。尚无 EVD 覆盖组。 |
| ADS-014 | DES-sponsored-ads | unknown | gap: 已实现政策码回显（广告与广告主视图）、暂停与下线原因，以及申诉：最新 revision 被拒或广告被下线时每个 revision 申诉一次（7106），视图给出 `appealable`，复审通过恢复投放、拒绝维持原状；集成测试 `TestAppealOncePerRevision` 与 Gateway 契约测试通过；e2e 已编写未运行。尚无 EVD 覆盖组。 |
| ADS-015 | DES-sponsored-ads | unknown | gap: 已实现：素材写私有桶，匿名读取限 `xbh-media`，过审后按 `ads/<sha256>` 不覆盖复制，发布前不进索引；单测通过，匿名访问未在联调中验证。尚无 EVD 覆盖组。 |
| ADS-016 | DES-sponsored-ads | unknown | gap: 已实现：`adpolicy.LandingURLValid` 在写入时以 7104 拒绝，并作为审核硬规则重复校验；单测 `TestLandingURLValid` 通过。尚无 EVD 覆盖组。 |
| ADS-017 | DES-sponsored-ads | unknown | gap: 已实现：酒精、博彩在所有演示市场由硬规则以对应政策码拒绝；单测通过。尚无 EVD 覆盖组。 |
| ADS-020 | DES-sponsored-ads | unknown | gap: 已实现：独立 `sponsored` 字段与 `afterPosition` 换算，帖子条目不变；单测 `TestRecommendWithAdSlotsPlacesSlotsByPosition` 通过。尚无 EVD 覆盖组。 |
| ADS-021 | DES-sponsored-ads | unknown | gap: 已实现：未声明 `adSlots=1` 不查询广告且响应无该字段；单测 `TestRecommendWithoutAdSlotsIsUnchanged` 通过。尚无 EVD 覆盖组。 |
| ADS-022 | DES-sponsored-ads | unknown | gap: 已实现：80 ms 超时与失败降级为无广告；单测 `TestRecommendDegradesWhenAdServiceFails` 通过。尚无 EVD 覆盖组。 |
| ADS-023 | DES-sponsored-ads | unknown | gap: 已实现：槽位含广告标识、广告主名称与 why（市场、场景、是否个性化）。尚无 EVD 覆盖组。 |
| ADS-024 | DES-sponsored-ads | unknown | gap: 已实现：UTC 自然日每广告 3 次，用户按 userId、匿名按会话哈希；Redis 集成测试 `TestRedisFrequencyHideAndIndex` 通过。尚无 EVD 覆盖组。 |
| ADS-025 | DES-sponsored-ads | unknown | gap: 已实现：选择规则不使用个人特征，why.personalized 恒为 false。尚无 EVD 覆盖组。 |
| ADS-026 | DES-sponsored-ads | unknown | gap: 已实现：隐藏与举报都写隐藏键，已认证用户 30 天、匿名仅当前会话；Redis 集成测试与 `TestReportAd` 单测通过；e2e 已编写未运行。尚无 EVD 覆盖组。 |
| ADS-027 | DES-sponsored-ads | unknown | gap: 已实现：recommend-mq 跳过 `target_type == ad`，ClickHouse `ad_daily_stats` 按（请求，目标类型，目标）去重；单测 `TestRecommendConsumerSkipsAdEvents` 通过。尚无 EVD 覆盖组。 |
| ADS-030 | DES-sponsored-ads | unknown | gap: 已实现：`POST /api/v2/ads/{adId}/report`，同一身份只计一次，批次内举报共用一个 report 任务且优先级随举报数提高，举报成立即下线，结论后到达的举报移入新批次；集成测试 `TestReportBatchesAndCarryOver`、`TestReportBatchRaisesPriority` 通过；e2e 已编写未运行。尚无 EVD 覆盖组。 |
| ADS-031 | DES-sponsored-ads | unknown | gap: 已实现：ad-mq 按 review-rpc 回扫代次（政策版本 + 生效种子变化计数）送回扫，机审判定违规先下发暂停结论再转人审，人审确认下线、否定恢复；单测 `TestRescannerSubmitsAdsApprovedBeforeTheGeneration`、`TestRescanJudgesViolationNotApproval` 与集成测试 `TestRescanPauseThenResumeOrOffline`、`TestRescanViolationPausesAndEscalates` 通过；投后任务不因未决 revision 作废属 `RVW-003` 澄清（见 PROP-20261001-post-serving-supersede）；e2e 已编写未运行。尚无 EVD 覆盖组。 |
| ADS-032 | DES-sponsored-ads | unknown | gap: 已实现：结论与暂停结论应用后立即更新索引，ad-rpc 资格缓存 10 秒；传播延迟未在联调中测量。尚无 EVD 覆盖组。 |
| ADS-040 | DES-sponsored-ads | unknown | gap: 已实现：广告主资源按本人过滤，越权返回不存在；私有资产只给本人或经审核平台授权的审核员；集成测试与 Gateway 契约测试通过。尚无 EVD 覆盖组。 |
| ADS-041 | DES-sponsored-ads | unknown | gap: 已实现：可解释规则选择（槽位、同广告主去重、当日最少优先、请求种子稳定排序），无竞价与计费；单测 `TestChooseRespectsCapAdvertiserAndPositions` 通过。尚无 EVD 覆盖组。 |
| ADS-050 | DES-sponsored-ads | unknown | gap: 已实现指标 `esx_ads_*`（含举报、申诉与回扫计数）与 ClickHouse 点击率视图；未在运行环境中观测。尚无 EVD 覆盖组。 |
| ADS-A01 | DES-sponsored-ads | unknown | gap: 新旧请求响应对比的 Gateway 单测通过；联调 e2e 未运行。尚无 EVD 覆盖组。 |
| ADS-A02 | DES-sponsored-ads | unknown | gap: 编辑过审广告的集成测试覆盖审核期旧快照、通过切换、被拒仍投旧快照；资质失效停投有单测；e2e 未运行。尚无 EVD 覆盖组。 |
| ADS-A03 | DES-sponsored-ads | unknown | gap: 广告服务超时与失败的 Gateway 单测通过；联调故障注入未运行。尚无 EVD 覆盖组。 |
| ADS-A04 | DES-sponsored-ads | unknown | gap: 用户与会话频控的 Redis 集成测试通过；e2e 未运行。尚无 EVD 覆盖组。 |
| ADS-A05 | DES-sponsored-ads | unknown | gap: recommend-mq 跳过广告事件单测通过；联调未验证特征与去重无变化。尚无 EVD 覆盖组。 |
| ADS-A06 | DES-sponsored-ads | unknown | gap: 落地页规则单测通过；未过审素材公开地址不可访问未在联调中验证。尚无 EVD 覆盖组。 |
| ADS-A07 | DES-sponsored-ads | unknown | gap: 回扫暂停、人审下线与恢复的单测和集成测试通过；根仓 e2e `test_rescan_pauses_violations_until_human_review` 已编写未运行，60 秒停投未在联调中测量。尚无 EVD 覆盖组。 |
| ADS-A08 | DES-sponsored-ads | unknown | gap: 年龄限制行业硬规则与缺资质不能送审的单测、集成测试通过；e2e 未运行。尚无 EVD 覆盖组。 |
| DISC-053 | DES-sponsored-ads | unknown | gap: 已实现：广告只出现在独立 `sponsored` 字段，帖子位置、游标与去重不变；Gateway 单测通过。尚无 EVD 覆盖组。 |
| REL-009 | DES-sponsored-ads | unknown | gap: 后端以 `target_type=ad` 接收并按（请求，目标类型，目标）在 ClickHouse 去重计数；客户端曝光上报属 W5。尚无 EVD 覆盖组。 |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出，路径均相对仓库根目录。新服务落地时同步更新本页与
`docs/knowledge/guides/architecture.md`。
