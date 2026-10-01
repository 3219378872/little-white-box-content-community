---
id: DES-sponsored-ads
layer: design
title: 付费广告与投放设计
status: active
owner: agent
updated_at: 2026-10-01
tracks:
- ADS-001
- ADS-002
- ADS-010
- ADS-011
- ADS-012
- ADS-013
- ADS-014
- ADS-015
- ADS-016
- ADS-017
- ADS-020
- ADS-021
- ADS-022
- ADS-023
- ADS-024
- ADS-025
- ADS-026
- ADS-027
- ADS-030
- ADS-031
- ADS-032
- ADS-040
- ADS-041
- ADS-050
- ADS-A01
- ADS-A02
- ADS-A03
- ADS-A04
- ADS-A05
- ADS-A06
- ADS-A07
- ADS-A08
- DISC-053
- REL-009
---

# 付费广告与投放设计

本页承接 [付费广告与投放规范](../spec/SPEC-sponsored-ads.md)，以及 `DISC-053`、`REL-009` 对发现与行为
链路的约束。审核流程见 [DES-review-platform](DES-review-platform.md)。截至 2026-10-01 尚无实现，逐条
状态见 [IMP-sponsored-ads](../implementation/IMP-sponsored-ads.md)。

## 组件与所有权

```text
Gateway
  /api/v2/advertisers*、/api/v2/ads*（广告主，需认证）-> ad-rpc
  /api/v2/feed/recommend?adSlots=1 -> feed-rpc（不变）并行 ad-rpc.GetSponsoredSlots（短超时）
  /api/v2/ads/:adId/hide、/report（访客）-> ad-rpc
ad-rpc
  -> 广告主、资质、广告、快照；送审 outbox；投放选择与频控
ad-mq
  -> 消费 review-decided（tag = ad_creative / advertiser_qualification），按 revision CAS 应用
  -> 维护投放索引；过审素材发布；ticker：对账、资质过期、暂停传播
media-rpc
  -> 广告素材与资质证件写入私有存储；过审后复制到公开内容寻址路径
behavior / pipeline
  -> targetType = ad 的曝光与点击进入 ClickHouse；recommend-mq 跳过非帖子事件
```

权威库为 `xbh_ad`，与内容库分离。ad-rpc 不提供把广告置为通过的接口，审核结论只经 `review-decided`
生效。

## 数据模型

| 存储 | 内容 | 关键约束 |
| --- | --- | --- |
| `advertiser` | 主体名称、经营市场、状态、风险等级 | 每个用户最多一个广告主；revision 与 approved_revision |
| `advertiser_qualification` | 市场、行业、证件媒体 ID、有效期、状态 | 证件只在私有存储；作为 `advertiser_qualification` 送审 |
| `ad` | 最新 revision、approved_revision、审核状态、投放状态、市场、行业、投放期 | 写操作带 expectedRevision 与幂等键 |
| `ad_snapshot` | 每个 revision 的文案、CTA、落地页、素材 sha256 与公开键 | 主键（ad_id, revision），写入后只读 |
| `ad_report` | 举报人身份、原因 | 同一身份对同一广告只计一次 |
| `event_outbox`、`idempotency` | 可靠投递与幂等 | 沿用 `pkg/outboxx/outbox.go`、`pkg/idempotencyx/idempotency.go` |

Redis 键均带 `ads:v1:` 前缀：

- 投放索引 `serving:<market>`：可投广告及其 approved_revision。
- 频控 `freq:<identity>:<yyyymmdd>`：广告 ID → 当日下发次数，Lua 内原子 HINCRBY 并设置 48 小时过期。
  现有验证码计数 INCR 与 EXPIRE 分两步，这里不沿用。
- 隐藏 `hidden:<identity>`：广告 ID → 到期时间，30 天。
- 请求结果 `req:<requestId>:<cursor 哈希>`：本页已选槽位，10 分钟，保证客户端重试得到相同广告且不重复计频。

identity 为 `u:<userId>`；匿名为 `s:<sessionId 哈希>`，只按会话计数，不使用 anonymousId，避免形成跨会话
匿名画像（`ADS-024`、`ADS-026`、`DISC-031`）。这些键只服务频控与用户屏蔽，不进入个性化特征。

## 状态与投放资格

审核状态针对最新 revision：`draft → pending_review → approved | rejected`，`rejected → appealing →
approved | rejected`。投放状态独立：`none → serving`（首次过审），`serving ⇄ paused`（回扫或质检违规先暂停，
人审否定后恢复），`paused | serving → offline`（人审确认违规或举报成立）。

投放资格在读取时计算（`ADS-010`）：存在 approved_revision、投放状态为 serving、受监管行业资质有效、
当前时间在投放期内。

- **编辑**（`ADS-011`）：修改文案、素材、落地页、行业或目标市场产生新 revision，同事务写快照与
  `review-submitted`。approved_revision 不变，旧过审快照继续投放。
- **应用结论**：通过时 `UPDATE ad SET approved_revision = r, review_status = 'approved' WHERE id = ? AND
  revision = r`，拒绝时只更新审核状态。影响 0 行说明已有更新的 revision，该结论只留审计（`RVW-003`）。
- **资质失效**（`ADS-002`）：ad-mq ticker 扫描到期资质，把依赖广告移出投放索引，并以
  `INDUSTRY.QUALIFICATION` 通知广告主（复用既有 SendNotification RPC）。

## 送审前校验

- **落地页**（`ADS-016`）：只接受 https；拒绝 userinfo、IP 字面量主机、localhost、`.local`、`.internal`
  等保留域名，以及超过 2,048 个字符的地址。系统不抓取落地页，不产生对外请求，也就没有 SSRF 面。
- **年龄限制行业**（`ADS-017`）：酒精与博彩行业作为审核平台的硬规则自动拒绝，并带对应政策码；控制台
  在创建时提前提示。
- **资质**（`ADS-001`、`ADS-002`）：受监管行业要求目标市场有效资质，缺失时拒绝送审并返回
  `INDUSTRY.QUALIFICATION`。

## 素材

现有媒体桶是 public-read（`app/media/rpc/internal/storage/s3.go`），上传后即可外链访问。广告素材与资质
证件改写到私有存储目标，存储接口补充读取与复制。审核员经 Gateway 鉴权后流式读取。过审后，ad-mq 把
素材按 `ads/<sha256>` 复制到公开路径：同一内容只写一次、不可覆盖（`ADS-015`）。根仓反代 CSP 只放行
同源图片，公开路径经 `/xbh-media/` 提供。

## 投放

- **契约**（`ADS-020`、`ADS-021`、`DISC-053`）：`/api/v2/feed/recommend` 新增可选请求参数 `adSlots`（1 表示
  支持）；响应新增可选字段 `sponsored`，每项包含 `slotId`、`afterPosition` 与 `ad`。`ad` 含 adId、
  revision、广告主名称、标题、正文、CTA、落地页与域名、图片、广告标识，以及 `why`（market、scene、
  personalized）。未声明参数时响应不出现该字段，帖子 `items`、位置、游标与去重完全不变。
- **组装**（`ADS-022`）：Gateway 在调用 feed-rpc 的同时以 80 ms 超时调用 `GetSponsoredSlots`。ad-rpc 返回
  页内相对的 afterIndex，Gateway 用本页帖子的 position 换算成 afterPosition，本页帖子不足时丢弃该槽位。
  广告调用失败只记指标并返回空数组；现有帖子 enrichment 失败会导致整页失败，广告路径必须与之隔离
  （`app/gateway/internal/logic/feed/get_recommend_feed_logic.go`）。
- **选择**（`ADS-041`）：每页最多 2 个槽位，默认在第 4 与第 12 条帖子之后，同一页同一广告主最多一个。
  在市场投放索引中排除已隐藏、当日已达 3 次和资格不满足的广告，优先当日下发最少者，同分按 requestId
  种子随机。关闭个性化的用户与匿名用户只按市场与场景选择，`why.personalized = false`（`ADS-025`）。
- **频控**（`ADS-024`）：在选中时计数，请求结果键保证重试不重复计数。
- **传播**（`ADS-032`）：暂停或下线事件由 ad-mq 立即移出投放索引；ad-rpc 进程内资格缓存不超过 30 秒，
  合计在 60 秒内对新请求生效。Redis 不可用时不返回广告。

## 行为事件

- 客户端以 `targetType: "ad"`、`targetId: adId` 上报曝光与点击；曝光沿用 50% 可见、连续 1 秒，position 取
  afterPosition（`REL-009`）。
- recommend-mq 目前会把任意目标的点击写进正反馈、挤占 `:recent` 列表，曝光去重键也不含目标类型
  （`app/recommend/mq/internal/store/behavior_store.go`）。改为在入口跳过 `target_type != post` 的事件，
  使广告事件不进入帖子特征、训练数据与帖子曝光去重（`ADS-027`）。
- ClickHouse 新增按（日期，广告，动作）的物化聚合；曝光按（request_id, target_type, target_id）去重后计数，
  与 `deploy/sql/xbh_analytics.sql` 现有的 90 天原始事件保留一致。
- 隐藏与举报不走遥测事件，而是走权威 REST（`ADS-026`、`ADS-030`）：隐藏写入隐藏键；举报写 `ad_report`
  并经 outbox 生成 `report` 审核任务，举报数提升优先级。

## 投后

- **回扫**（`ADS-031`）：政策版本或生效种子库变更后，ad-mq 枚举投放中的过审快照，送 `rescan` 任务。
  机审判定违规时先暂停，再进入人审；人审确认后下线，否定后恢复。
- **质检违规**：处置路径同回扫。
- **申诉**（`ADS-014`）：每个被拒或被下线的 revision 可申诉一次，状态转为 `appealing`，复审由不同审核员
  处理，结论为最终结论。

## 广告主接口

需认证的 REST：申请广告主、查询本人广告主、上传与提交资质、创建与编辑广告（expectedRevision 与幂等键）、
列表与详情（状态、政策码、过审快照）、申诉、上传素材到私有存储。广告主只能访问自己的资源，越权统一
返回不存在（`ADS-040`）。错误码在 `pkg/errx` 新增广告号段。

## 观测

指标前缀 `esx_ads_`：投放请求（结果）、下发槽位、频控拦截、降级（原因）、暂停与下线的传播延迟。点击率由
ClickHouse 聚合计算（`ADS-050`）。

## 失败模式

| 故障 | 行为 |
| --- | --- |
| ad-rpc 超时或不可用 | 推荐流正常返回，不含广告 |
| Redis 不可用 | 不返回广告；广告主写操作不受影响 |
| review-decided 积压 | 新版本停留审核中，旧过审快照继续投放 |
| 素材复制失败 | 结论已应用但不进入投放索引，ad-mq 重试并告警 |
| 资质到期扫描延迟 | ticker 间隔 1 分钟，最长延迟受告警约束 |

## 取舍

- **独立 `sponsored` 字段 vs 混入 `items`**：现有前端遇到非帖子条目会整页失败，混入还会改变 position、
  快照 offset 和去重语义；独立字段兼容旧客户端，代价是客户端要自行合并展示。
- **下发时计频 vs 曝光时计频**：下发时计数确定、可在服务端强制，代价是未曝光也计入；结合请求结果键
  避免重试重复计数。
- **页内相对槽位 vs 全局槽位**：不需要跨页状态，也不碰推荐快照；代价是不同页大小下广告密度不完全一致。

## 验收策略

- 单元：状态机与资格计算、CAS 应用、URL 校验、槽位换算、频控 Lua、身份键。
- 集成：outbox → review → 结论回写；资质到期停投；投放索引传播。
- 故障注入：ad-rpc 超时与 Redis 故障下推荐流仍成功（`ADS-A03`）。
- e2e（根仓 pytest）：新旧客户端响应对比（`ADS-A01`），编辑过审广告（`ADS-A02`），频控（`ADS-A04`），
  广告事件隔离（`ADS-A05`），私有素材与落地页（`ADS-A06`），回扫暂停（`ADS-A07`），年龄限制行业（`ADS-A08`）。

## 分期

| 周 | 范围 |
| --- | --- |
| W1 | xbh_ad、广告主与广告写路径、快照与送审、结论应用、对账 |
| W2 | 落地页与行业校验、资质流程、私有素材 |
| W4 | 投放索引、`GetSponsoredSlots`、Gateway 合并与能力参数、频控、recommend-mq 过滤、ClickHouse 聚合 |
| W5 | 前端卡片、控制台与工作台联调 |
| W6 | 回扫、举报、申诉、证据 |
