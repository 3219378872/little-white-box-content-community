---
id: DES-review-platform
layer: design
title: 审核平台设计
status: active
owner: agent
updated_at: 2026-10-01
tracks:
- RVW-001
- RVW-002
- RVW-003
- RVW-004
- RVW-010
- RVW-011
- RVW-012
- RVW-013
- RVW-014
- RVW-015
- RVW-016
- RVW-017
- RVW-018
- RVW-020
- RVW-021
- RVW-022
- RVW-023
- RVW-024
- RVW-025
- RVW-030
- RVW-040
- RVW-041
- RVW-050
- RVW-051
- RVW-060
- RVW-061
- RVW-A01
- RVW-A02
- RVW-A03
- RVW-A04
- RVW-A05
---

# 审核平台设计

本页承接 [审核平台规范](../spec/SPEC-review-platform.md)。广告业务语义、政策码与投放见
[DES-sponsored-ads](DES-sponsored-ads.md)。W1～W3 与 W6（政策回扫、申诉与举报任务）已于 2026-10-01 落地；
逐条状态见 [IMP-review-platform](../implementation/IMP-review-platform.md)，实现完成不等于验证完成。

机审的工程骨架参考 TikTok [Filter-And-Refine](https://arxiv.org/html/2507.17204v1) 的 Router → Ranker
级联：低成本召回过滤大部分内容，高成本模型只对候选 issue 精排。论文面向先发后审，Router 放过等于不处置；
广告是先审后投，Router 漏召会直接变成漏放，所以本设计把「召回」与「放行」分开，并用抽样质检估计漏放。

## 组件与所有权

```text
业务服务（ad-rpc）
  -> 同事务写业务状态 + outbox: review-submitted（tag = bizType，携带快照）
review-worker
  -> 消费送审事件，按唯一键幂等建任务
  -> 机审级联：指纹复用 -> 硬规则 -> Router -> Ranker -> 决策
  -> 自动结论：同事务写结论 + outbox: review-decided（tag = bizType）
  -> 灰区与降级：进入人审队列
  -> ticker：质检抽样、持有回收告警、政策回扫
review-rpc
  -> 审核员领取 / 续期 / 放弃 / 提交（租约 + fencing）
  -> 任务与证据读取、种子提名与确认、角色查询、EnsureSubmitted 对账入口
moderation-infer（Python sidecar，模型占位）
  -> Ranker：每个 issue 的 Yes/No 概率
embedding 服务（既有）+ Milvus 种子集合
  -> Router：文本向量检索
```

- 首批业务类型：`ad_creative`（广告素材 revision）与 `advertiser_qualification`（广告主主体与资质）。
  资质对象是「广告主主体 + 全部未失效资质」一个整体：修改资料或新增资质都前移广告主 revision 并整体送审，
  以首个经营市场为审核市场，只由 qualification_reviewer 领取。业务方只依赖两类事件和 `EnsureSubmitted`，
  不了解审核内部阶段。
- 代码位置：共享领域 `app/review/internal`（快照、政策、存储、级联、召回、精排客户端、指标），
  review-rpc `app/review/rpc`，review-worker `app/review/worker`，角色运维脚本 `app/review/rolectl`，
  精排占位 sidecar `algorithm/moderation_infer`（契约 `proto/moderation/moderation.proto`）。
- 审核平台不提供「把对象置为通过」的接口；结论只经 `review-decided` 事件生效（`RVW-051`）。
- 权威库为 `xbh_review`，与业务库分离；投递沿用 `pkg/outboxx/outbox.go` 的事务发件箱。

## 数据模型

| 表 | 作用 | 关键约束 |
| --- | --- | --- |
| `review_task` | 审核任务与人审队列 | 唯一键（biz_type, object_id, object_revision, purpose, purpose_key）；租约字段与 attempt |
| `review_snapshot` | 冻结快照 | 主键为规范化 JSON 的 sha256，写入后只读 |
| `review_stage_result` | 每个机审阶段的版本、耗时、状态与输出 | 含 shadow 标记，只追加 |
| `review_decision` | 结论 | 只追加；记录来源、政策版本、提交时的 lease_generation |
| `verdict_cache` | 指纹复用 | 主键（snapshot_hash, market, policy_version） |
| `review_seed` | 种子库权威记录 | candidate → active → retired；提名人与确认人不同 |
| `reviewer` | 角色、市场与语言授权 | 运维脚本写入；每次请求实时读取 |
| `audit_log` | 审核动作与配置激活审计 | 应用账号只授予 INSERT/SELECT |
| `event_outbox` | 可靠投递 | 与业务写同事务 |

`purpose` 取 `initial`、`qa`、`appeal`、`report`、`rescan`；`purpose_key` 对回扫是回扫代次（政策版本 +
生效种子变化计数），对举报是举报批次，其余为空。同一对象 revision 的首次送审只有一条 `initial` 任务
（`RVW-001`），质检、申诉、举报与回扫各自成任务。

作废分两类（`RVW-003`）：

- **送审与申诉**（`initial`、`appeal`）审的是 revision 能否投放。对象出现新 revision 时，同对象旧
  revision 的这类未决任务在同一事务内置为 `superseded`；乱序到达的旧 revision 送审直接以 `superseded` 落库。
- **投后任务**（`qa`、`report`、`rescan`）审的是正在投放的过审快照。新 revision 审核期间旧快照继续投放
  （`ADS-011`），因此它们不因更新的未决 revision 作废，而在更新的 revision 过审（送审或申诉通过）的同一
  事务内作废；此后到达的旧快照投后任务直接以 `superseded` 落库。若新 revision 审核期间就作废它们，回扫暂停
  的广告会卡在暂停（新版本被拒时新旧版本都无法投放），举报也会静默丢失。

任务状态：`machine_pending → machine_running → decided | human_pending`；
`human_pending → claimed → decided | human_pending`（持有过期或放弃）；任一未决状态 → `superseded`。

## 快照与规范化

送审事件携带业务快照：文案、CTA、落地页地址、素材 sha256 与公开键、市场、语言、行业、提交主体。
worker 先做规范化再算哈希：键排序、Unicode NFC、去除零宽字符、全角半角折叠，避免用不可见字符或变体
绕过规则与指纹（`RVW-002`）。快照按哈希写入后不可修改。

## 机审级联

| 阶段 | 输入 | 输出与处置 | 失败时 |
| --- | --- | --- | --- |
| S1 指纹复用 | (snapshot_hash, market, policy_version) | 命中人工通过或任意拒绝即出结论；保护期内不复用通过（`RVW-015`） | 视为未命中 |
| S2 硬规则 | 规范化文本、落地页地址、素材哈希、行业、资质 | 命中即拒绝，或强制人审 | 转人审 |
| S3 Router | 文本向量 | 每个 issue 的最高相似度，≥ τ_route 的成为候选 | 全部 issue 进入 S4（`RVW-011`） |
| S4 Ranker | 候选 issue 与按 issue 版本化的提示模板 | 每个 issue 的 pYes / pNo | 转人审 |
| S5 决策 | 上述结果与政策版本 | 自动拒 / 自动过（抽样质检）/ 人审 | — |

- **Router**：调用既有 embedding 服务（`proto/embedding/embedding.proto`，384 维多语言文本向量），在
  Milvus 种子集合中按市场与状态过滤检索 top-k。集合沿用 `_current` 别名切换版本，迁移方式同
  [向量投影迁移](../guides/vector-projection-migration.md)。版本串形如 `seedbank@<n>+<embedding 版本>`。
  现有 Milvus 封装位于 internal 包且没有按向量检索的接口，review 内新写检索适配
  （`app/review/internal/router`）。当前使用物理集合 `review_seed_bank_v1`（内积度量 + 归一化向量即余弦相似度），
  尚未接入 `_current` 别名切换；集合可由权威记录重建。市场没有生效种子时视为召回无覆盖，全部 issue 进入精排，
  避免空种子库让召回把一切放行。
- **图片**：图片向量 Router 是占位接口，当前只按 sha256 精确匹配黑样本。素材含未经人工确认的图片时
  视为该维度召回不可用，不满足自动通过条件，按 `RVW-011` 转人审；图片 sha256 已在人工通过的快照中
  出现过时，视为已确认。
- **Ranker**：新 proto 定义 `Score(task_id, market, language, text, media_sha256[], issues[])`，返回每个
  issue 的 `p_yes`、`p_no` 与 `model_version`，对应论文 Algorithm 1 的 Yes/No 单 token 概率。Go 侧校验
  每个请求 issue 都有分数、概率在 0～1 且两者之和接近 1、版本非空，否则按无效结果转人审。超时 2 秒，
  worker 异步执行，不在用户请求路径上。
- **占位模型**：`moderation-infer` 默认实现 `stub-v0` 对所有 issue 返回 0.5，使有候选的任务全部落入
  灰区进人审；仅在 `MODERATION_FIXTURE_ENABLED=1` 时按文案中的 `[[fixture:<issue>=<p>]]` 标记返回指定分数，
  版本串为 `stub-v0+fixture`。版本串带 `stub`，满足 `RVW-018`。
  降级标注、超时与故障注入沿用 `app/recommend/rpc/internal/logic/rank.go` 与
  `app/recommend/rpc/internal/logic/inference_fault_injection_test.go` 的做法。
- **决策**（`RVW-012`、`RVW-013`）：命中硬规则直拒；任一候选 issue 的 pYes ≥ τ_reject 且该（issue，
  市场）允许自动拒绝则直拒；全部候选 pYes < τ_pass、市场与行业允许自动通过、广告主已有 3 次以上送审
  且无图片维度降级时自动通过；其余进人审，优先级由最高 pYes、截止时间与举报数决定。
- **prompt 注入**：广告文案由攻击者控制，只作为 Ranker 请求的数据字段传递；契约只接受固定的 Yes/No
  概率，注入最多影响一个概率值，再由阈值、硬规则和高风险行业强制人审兜底。
- **影子**（`RVW-016`）：worker 可同时加载 active 与 shadow 两个政策版本。shadow 在 active 路径之后运行，
  结果写入 `review_stage_result` 并标记 shadow，不写结论、不改任务。online_infer 已有的 shadow 只打日志，
  这里改为落库以便对比。
- **复现**（`RVW-017`）：每阶段记录组件版本、输入引用、输出与耗时；配合快照与政策版本可在离线任务中
  复放决策路径，用历史人工结论评估新阈值。

## 政策配置

政策按版本发布为配置文件（policy as code），每个版本包含：启用的 issue、各市场的行业处置（允许、
需资质、禁止）、各（issue，市场）的 τ_route / τ_pass / τ_reject 与自动处置开关、硬规则（按语言的
关键词与正则、域名黑名单、URL 规则）。规则与阈值的变更经版本库评审，提交历史就是变更记录；worker
启动激活某版本时写一条 `audit_log`（含配置哈希）。运行期变更（种子、角色）直接写 `audit_log`
（`RVW-025`）。

演示市场为 `US`、`DE`、`ID`，语言分别为 en、de、id。演示矩阵只用于验证按市场分派的机制，处置与
数值不对应任何真实法规或 TikTok 现行规则：

| 行业 | US | DE | ID |
| --- | --- | --- | --- |
| 成人、危险商品、政治 | 禁止 | 禁止 | 禁止 |
| 酒精、博彩（平台无年龄能力） | 禁止 | 禁止 | 禁止 |
| 金融、医疗 | 需资质，禁用自动通过 | 需资质，禁用自动通过 | 需资质，禁用自动通过 |
| 体重管理 | 允许，禁用自动通过 | 需资质 | 禁止 |
| 其他 | 允许 | 允许 | 允许 |

## 人审队列

- **领取**（`RVW-020`、`RVW-022`）：在事务内按审核员的市场与语言授权、排除名单过滤，
  `ORDER BY priority DESC, deadline_ms ASC, id ASC LIMIT 1 FOR UPDATE SKIP LOCKED`；`claimed` 且租约
  已过期的任务视同待领取。领取时 `lease_generation + 1`，持有 10 分钟。做法沿用
  `app/assistant/internal/store/sql_runs.go` 的 Claim 与 `app/assistant/internal/lease/lease.go`。
- **fencing**（`RVW-021`）：续期、放弃、提交都带（task_id, reviewer, lease_generation），SQL 条件同时
  要求 `status = 'claimed'` 且租约未过期；影响 0 行时区分「持有已失效」与「任务已作废」返回。
- **提交**：一个事务内写 `review_decision`、更新任务、写 `review-decided` outbox 与 `audit_log`；客户端
  幂等键经 `pkg/idempotencyx/idempotency.go`（scope `review:decision`）。
- **attempt**：每次领取加一；超过 5 次的任务提升优先级（+100，上限 1000）并计入
  `esx_review_claim_attempts_exceeded_total`，避免 agent_run 式的无限重领。
- **证据展示**（`RVW-023`）：读取快照、各阶段输出与政策定义；资质证件经 Gateway 鉴权后从私有存储流式
  读取，只对具备资质审核权限的审核员开放。
- **质检与申诉**（`RVW-014`、`RVW-024`）：自动通过按 ≥ 5% 抽样建 `qa` 任务，质检判定违规时业务方
  立即下线。申诉由业务方以 `appeal` 送审，直接进入人审，不跑机审；送审时取该 revision 最近一次拒绝结论
  作为原结论（`source_task_id`），并把其决策人写入排除名单，机审作出的拒绝没有需要排除的人。工作台展示
  原结论，申诉改判计入 `esx_review_appeal_overturns_total`。
- **举报**（`ADS-030`）：业务方以 `report` 送审，直接进入人审，优先级由业务方按批次举报数给出；同一批次
  （`purpose_key`）重复送审只提高未决任务的优先级，不新建任务。

## 政策回扫

回扫（`ADS-031`）由业务方触发：审核平台不了解业务对象全集，只经 `GetRescanGeneration` 给出当前代次
`<政策版本>+seeds@<n>`，n 为生效种子集合的变化计数（每次确认、每次停用已生效种子各加一，单调递增），以及
该代次的生效时间：政策版本首次以 active 模式激活的时间（来自 `policy.activate` 审计）与最近一次种子变化
加 60 秒（覆盖向量集合每 30 秒的同步延迟）中较晚者。review-rpc 的政策版本尚未被 worker 激活时返回
`ready=false`，避免按旧版本重审。业务方以代次作为 `purpose_key` 送 `rescan` 任务。

回扫走机审，但判定的是「是否违规」而不是「能否放行」：

- 硬规则命中，或任一候选 issue 的分数达到拒绝阈值（不论该 issue 是否允许自动拒绝）即判定违规；
- 强制人审规则或灰区分数转人审核实，不暂停投放；精排失败照常转人审（`RVW-011`）；
- 其余视为未发现违规，以机审通过结案。行业禁用自动通过、首次送审保护期与图片未确认是授予自动通过的
  条件，不适用于已过审在投的快照；回扫不复用指纹，结论也不写入指纹复用表。

判定违规时，worker 在同一事务内把任务转人审（原因 `rescan-violation`，优先级不低于 90）、写审计，并经
outbox 下发 `interim=true` 的 `review-decided` 暂停结论。暂停结论不写 `review_decision`，只要求业务方先停投；
该任务的人审结论是最终结论：拒绝即确认违规下线，通过则恢复投放。

## 种子库

审核员提交拒绝时可提名候选种子（取快照文案，以首个政策码为 issue）；具备政策管理权限的另一人确认后置为
active，同一人提名与确认会被拒绝（`RVW-030`）。worker 每 30 秒按 `index_state` 把 active 种子写入集合、
把 retired 种子移出集合。停用时先改权威记录，Router 检索后按权威状态复核，保证停用后新任务不再命中。

## 结论下发与一致性

- `review-decided` 载荷为 bizType、objectId、revision、taskId、purpose、purposeKey、verdict、policyCodes、
  policyVersion、source、decidedAt，以及只用于回扫暂停的 interim；消息 tag 为 bizType，业务方按 tag 订阅。
- 业务方按（objectId, revision）CAS 应用、按 taskId 幂等；revision 单调递增，乱序到达的旧结论只留审计
  （`RVW-040`）。outbox 至少一次投递即可，不需要分布式事务。
- 对账（`RVW-041`）：业务方 ticker 找出送审超过 5 分钟仍未登记任务的对象，调用 `EnsureSubmitted`
  幂等补建任务。审核平台不需要了解业务对象全集。

## 权限与安全

- 角色为 reviewer、qa、policy_admin、qualification_reviewer，连同市场与语言授权存 `reviewer` 表，由
  运维脚本 `app/review/rolectl` 授予与撤销并写审计。Gateway 每次请求实时查询，不进 JWT（`pkg/jwtx/jwt.go` 只有用户标识，
  access token 有效期 1800 秒），撤销对下一次请求生效（`RVW-050`）。
- 内部 HMAC（`pkg/rpcx/transport.go`）只证明请求来自内部服务，不识别调用方、不签 body；因此结论不提供
  RPC 写入口，只经事件通道生效。
- `moderation-infer` 只绑 127.0.0.1，不暴露 reload；现有 Python sidecar 的 gRPC 无鉴权。
- 业务日志不记录广告正文与资质证件内容，与 `REL-022` 一致。

## 观测

指标前缀 `esx_review_`：送审到结论耗时（label：machine / human）、结论计数（来源、结论）、人审积压与
最老任务年龄、质检不一致与申诉改判计数、回扫暂停计数、指纹命中、各阶段耗时与降级计数（阶段、原因）。机审自动化率由
结论计数推导（`RVW-060`）。送审到首次结论的 P95 对照 24 小时目标如实报告（`RVW-061`）。

## 失败模式

| 故障 | 行为 |
| --- | --- |
| embedding 或 Milvus 不可用 | Router 降级，全部 issue 进 Ranker，阶段记录 `router-unavailable` |
| moderation-infer 超时、不可用或返回无效 | 转人审，记录 `ranker-timeout` / `ranker-unavailable` / `ranker-invalid` |
| 政策配置加载失败 | worker 拒绝启动，不使用旧缓存猜测 |
| worker 崩溃 | `machine_running` 任务的租约到期后被其他 worker 重领，阶段结果按（task, stage, version）幂等 |
| 审核员中途离线 | 持有 10 分钟后过期，任务回到队列；旧持有者提交被 fencing 拒绝 |
| outbox 投递积压 | 业务对象停留审核中，对账与 outbox 指标告警；不影响已投放的过审快照 |

## 取舍

- **通用平台 vs 内嵌在广告服务**：多一跳异步和一个服务，换来业务与审核解耦，后续社区内容等业务类型
  只需接入事件与快照。
- **拉模式领取 vs 推送分配**：拉模式天然支持按授权过滤和 SKIP LOCKED 并发，代价是优先级完全依赖排序键。
- **MySQL 租约 vs Redis 锁**：结论与租约在同一库同一事务内，fencing 简单可证；吞吐上限足够人审量级。
- **配置化政策 vs 数据库规则**：版本评审、可复现、易回滚；代价是改规则要发版，热更新只到「切换版本」。
- **不抓取落地页**：避免 SSRF 与对外请求面，落地页内容由人审判断，代价是无法自动发现落地页伪装。

## 验收策略

- 单元：规范化与快照哈希、决策矩阵、URL 规则、状态机、Ranker 响应校验。
- 集成（`//go:build integration`，testutil 容器）：唯一键幂等、supersede、租约竞争与 fencing、outbox 投递。
- 故障注入：Router 与 Ranker 的超时、不可用、无效输出（`RVW-A02`）。
- e2e（根仓 pytest）：送审 → 机审 → 领取 → 提交 → 业务状态生效；质检与申诉换人；举报复审；回扫暂停
  后人审下线或恢复；角色撤销。

## 分期

| 周 | 范围 |
| --- | --- |
| W1 | xbh_review、任务与快照、人审领取 / fencing / 提交、结论事件、角色与审计、对账入口（已实现） |
| W2 | 指纹复用、硬规则、政策配置与决策矩阵、降级标注、质检抽样、指标（已实现） |
| W3 | Router（embedding + Milvus 种子集合）、Ranker proto 与占位 sidecar、影子运行、种子流程（已实现） |
| W6 | 政策回扫（代次、违规暂停与人审）、申诉与举报任务、投后任务作废规则（已实现） |

## 风险

- 本地 Milvus 为 v2.2.8（`deploy/docker-compose.middleware.yml`），Go SDK 为 v2.4.2，测试容器为 v2.5.6；
  种子集合的插入、删除与按向量检索已在 v2.5.6 测试容器上通过集成测试，v2.2.8 尚待联调栈验证。
- `just infer-up` 只等待在线推理端口，不等待 embedding；worker 必须把 embedding 暂不可用当作降级处理。
- 占位模型下自动化率接近 0 是预期结果，不能据此评价级联效果。
