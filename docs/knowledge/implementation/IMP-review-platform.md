---
id: IMP-review-platform
layer: implementation
title: 审核平台实现映射
status: active
owner: agent
code_paths:
- app/review/internal
- app/review/rpc
- app/review/worker
- app/review/rolectl
- algorithm/moderation_infer
- proto/review/review.proto
- proto/moderation/moderation.proto
- pkg/event/review.go
- pkg/adpolicy
- app/gateway/internal/logic/review
- deploy/sql/xbh_review.sql
updated_at: 2026-10-01
---

# 审核平台实现映射

审核平台 W1～W3 与 W6 已于 2026-10-01 实现（任务与快照、机审级联、人审持有与 fencing、种子库、角色与审计、
结论事件；W6 为回扫代次与违规暂停、申诉与举报任务、投后任务作废规则）。全部条款在取得当前提交上的有效 EVD 覆盖组前保持
`unknown`；gap 写明已实现内容、已执行测试与仍缺的验证。

| requirement | design | state | evidence or gap |
| --- | --- | --- | --- |
| RVW-001 | DES-review-platform | unknown | gap: 已实现：`store.Ingest` 唯一键幂等，并发同键回读胜出任务；集成测试 `TestIngestIsIdempotentUnderConcurrency` 通过。尚无 EVD 覆盖组。 |
| RVW-002 | DES-review-platform | unknown | gap: 已实现：`snapshot.Freeze` 规范化（NFC、零宽字符、全半角、空白）后 sha256，快照只写一次；单测 `TestFreezeIgnoresInvisibleAndWidthVariants` 通过。尚无 EVD 覆盖组。 |
| RVW-003 | DES-review-platform | unknown | gap: 已实现：新 revision 同事务作废旧 revision 的未决送审与申诉任务，乱序旧 revision 直接以 superseded 落库，作废任务的提交与续期返回 7002；投后任务（质检、举报、回扫）在更新的 revision 过审时才作废，与字面规则不同，澄清见 PROP-20261001-post-serving-supersede（待人类决定）；集成测试 `TestNewRevisionSupersedesPendingTasks`、`TestPostServingTasksFollowTheServingRevision` 通过。尚无 EVD 覆盖组。 |
| RVW-004 | DES-review-platform | unknown | gap: 已实现：结论记录来源、政策版本与时间，拒绝必须带 `pkg/adpolicy` 政策码（`validateVerdict`、`ReviewDecidedEvent.Validate`）。尚无 EVD 覆盖组。 |
| RVW-010 | DES-review-platform | unknown | gap: 已实现：`app/review/internal/cascade` 按指纹→硬规则→Router→Ranker→决策执行；单测 `TestAutoPassRequiresEveryCondition` 通过。尚无 EVD 覆盖组。 |
| RVW-011 | DES-review-platform | unknown | gap: 已实现：召回不可用或无种子覆盖时全部 issue 精排，精排超时/不可用/无效转人审并记录原因；单测 `TestDegradationNeverLoosensDecisions` 通过。尚无 EVD 覆盖组。 |
| RVW-012 | DES-review-platform | unknown | gap: 已实现：自动通过同时要求无硬规则、候选分数低于通过阈值、行业允许、前 3 次送审之后且图片已确认；单测通过。尚无 EVD 覆盖组。 |
| RVW-013 | DES-review-platform | unknown | gap: 已实现：仅硬规则或允许自动拒绝且分数达拒绝阈值时自动拒绝；单测 `TestRankerScoresDriveRejectAndGrayZone` 通过。尚无 EVD 覆盖组。 |
| RVW-014 | DES-review-platform | unknown | gap: 已实现：自动通过按政策比例（≥5%）抽样建 qa 任务，质检拒绝使广告下线；单测 `TestAutoApproveCreatesSampledQA` 与集成测试通过；质检不一致率未在联调中观测。尚无 EVD 覆盖组。 |
| RVW-015 | DES-review-platform | unknown | gap: 已实现：复用表按（哈希，市场，政策版本）命中，通过只来自人工或质检且受保护期约束；单测 `TestFingerprintReuse` 与集成测试通过。尚无 EVD 覆盖组。 |
| RVW-016 | DES-review-platform | unknown | gap: 已实现：worker 可加载影子政策，影子结果只写 shadow 阶段记录；单测 `TestShadowRunOnlyRecordsStages` 通过；当前配置未启用影子版本。尚无 EVD 覆盖组。 |
| RVW-017 | DES-review-platform | unknown | gap: 已实现：每阶段记录组件版本、耗时、输出与原因（`review_stage_result`）；离线复放工具尚未提供。尚无 EVD 覆盖组。 |
| RVW-018 | DES-review-platform | unknown | gap: 已实现：占位精排版本串为 `stub-v0`（fixture 为 `stub-v0+fixture`）；sidecar 单测通过。尚无 EVD 覆盖组。 |
| RVW-020 | DES-review-platform | unknown | gap: 已实现：`FOR UPDATE SKIP LOCKED` 领取，持有 10 分钟可续期；集成测试 `TestClaimCompetitionAndFencing` 通过。尚无 EVD 覆盖组。 |
| RVW-021 | DES-review-platform | unknown | gap: 已实现：续期、放弃、提交校验（持有者，代次，租约），失效返回 7001、作废 7002、已决 7004；集成测试通过。尚无 EVD 覆盖组。 |
| RVW-022 | DES-review-platform | unknown | gap: 已实现：按角色、市场、语言过滤并按优先级、截止时间、ID 排序；集成测试 `TestClaimRespectsScopeAndQAExclusion` 通过。尚无 EVD 覆盖组。 |
| RVW-023 | DES-review-platform | unknown | gap: 已实现：`GetTask` 展示快照、阶段证据与原结论，证件读取经 `AuthorizeEvidenceMedia` 限资质审核员；Gateway 契约测试通过，前端展示属 W5。尚无 EVD 覆盖组。 |
| RVW-024 | DES-review-platform | unknown | gap: 已实现：质检任务排除原决策人；申诉直接进入人审，以该 revision 最近一次拒绝为原结论并排除其决策人；集成测试 `TestClaimRespectsScopeAndQAExclusion`、`TestAppealExcludesOriginalDecider` 通过；根仓 e2e（test_ads_review，本地联调栈，后端 9acc84f4363b、前端 f55b433bf8ba）通过。尚无 EVD 覆盖组。 |
| RVW-025 | DES-review-platform | unknown | gap: 已实现：领取、续期、放弃、提交、种子与角色变更、政策激活写 `audit_log`；集成测试核对审计条数；应用账号只授予该表 INSERT/SELECT 需在根仓编排验证。尚无 EVD 覆盖组。 |
| RVW-030 | DES-review-platform | unknown | gap: 已实现：候选种子需不同的政策管理员确认，停用后召回按权威状态复核；集成测试 `TestSeedRequiresSecondPerson` 与 Milvus 集成测试通过。尚无 EVD 覆盖组。 |
| RVW-040 | DES-review-platform | unknown | gap: 已实现：结论、任务更新与 outbox 同事务，ad-mq 按 revision CAS 应用；集成测试 `TestEditApprovedAdKeepsServingOldSnapshot` 通过。尚无 EVD 覆盖组。 |
| RVW-041 | DES-review-platform | unknown | gap: 已实现：ad-mq 每分钟找出送审或申诉超过 5 分钟且未登记任务的对象调用 `EnsureSubmitted`；集成测试 `TestReconcileFindsStalePendingAds`、`TestAppealOncePerRevision` 通过，补送链路未在联调中验证。尚无 EVD 覆盖组。 |
| RVW-050 | DES-review-platform | unknown | gap: 已实现：review-rpc 每次请求实时读取 `reviewer` 表，角色只经 `app/review/rolectl` 授予与撤销；集成测试 `TestGrantAndRevokeRolesAreAudited` 通过。尚无 EVD 覆盖组。 |
| RVW-051 | DES-review-platform | unknown | gap: 已实现：review-rpc 与 ad-rpc 均无置为通过的写接口，结论只经 `review-decided` 生效。尚无 EVD 覆盖组。 |
| RVW-060 | DES-review-platform | unknown | gap: 已实现指标 `esx_review_*`（耗时、结论、积压、最老任务、质检不一致、申诉改判、回扫暂停、指纹、降级）；未在运行环境中观测。尚无 EVD 覆盖组。 |
| RVW-061 | DES-review-platform | unknown | gap: 已提供送审到结论耗时直方图；占位模型与演示流量不能代表真实时效，P95 未测量。尚无 EVD 覆盖组。 |
| RVW-A01 | DES-review-platform | unknown | gap: 重复送审、作废与乱序已有集成测试；结论重复与乱序投递已有 ad 侧集成测试；端到端 e2e 未运行。尚无 EVD 覆盖组。 |
| RVW-A02 | DES-review-platform | unknown | gap: 召回与精排超时、不可用、无效输出的注入单测通过；联调栈故障注入未运行。尚无 EVD 覆盖组。 |
| RVW-A03 | DES-review-platform | unknown | gap: 竞争领取与过期接手后旧持有者三类操作被拒的集成测试通过；e2e 未运行。尚无 EVD 覆盖组。 |
| RVW-A04 | DES-review-platform | unknown | gap: 影子不改结论、指纹三项一致与通过来源限制的单测通过；e2e 未运行。尚无 EVD 覆盖组。 |
| RVW-A05 | DES-review-platform | unknown | gap: 质检与申诉换人、角色撤销与审计已有集成测试；申诉换人的根仓 根仓 e2e（test_ads_review，本地联调栈，后端 9acc84f4363b、前端 f55b433bf8ba）通过。尚无 EVD 覆盖组。 |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出，路径均相对仓库根目录。新服务落地时同步更新本页与
`docs/knowledge/guides/architecture.md`。
