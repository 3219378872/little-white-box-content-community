---
id: PROP-20260827-agent-spec-closure
layer: proposal
title: 08-27 Agent 扩张后意图与规范收口
status: superseded
owner: agent
target_layer: spec
upstream:
  - INT-content-community-backend
  - SPEC-community-core
  - SPEC-content-discovery
  - SPEC-grounded-assistant
  - SPEC-assistant-agent-mode
  - SPEC-agent-memory
  - SPEC-agent-watch
  - SPEC-feedback-reliability
---

# 08-27 Agent 扩张后意图与规范收口

> 2026-08-29 superseded：本提案针对旧双模式 Agent 的收口选项，已被人类批准的 Hermes 式长期
> Agent 重构整体接替。当前正式语义见 `SPEC-assistant-agent`、重写后的 Memory/Watch 与可靠性规范。

本提案不是正式上游。未经人类逐项接受前，不得改写已批准 `INT-*` / `SPEC-*`，
也不得被设计或实现引用为约束。

审查对象：`INT-content-community-backend` 与七份已批准规范。客户端漂移列为跨仓跟进，
不在本仓直接改规格。楼中楼读取仍由
[PROP-20260822-comment-reply-thread](PROP-20260822-comment-reply-thread.md) 处理，
本提案只要求补齐评论对象本身。

---

# 观察到的问题

08-27 将 Agent 从「授权后发帖」扩成搜索综述、结构化记忆、可解释推荐与条件追踪后，
意图边界清楚，但若干已批准条款互相冲突或无法验收。当前 `approved` 状态下，
设计与实现无法同时满足全部条款。

阻断级：

1. `REL-033` 将 Assistant 完成 p95 定为 12 秒；`AGNT-030` 默认 8～12 步模型轮次，
   `AGNT-021` 删帖确认默认等待 120 秒。`REL-032` 完成延迟算到终止事件。Agent 会话
   会把月度完成 SLO 打穿，且规范未把确认等待排除出分母。
2. `ASST-042` 禁止在未重批本规范的情况下增加来源类型；`ASST-010` 要求每个事实段落
   使用社区证据；`AGNT-012` 又要求网页结果用独立来源类型、不得当社区证据。
   `web_search` 在白名单中，却几乎没有合法的对外事实用法。
3. 意图非目标写受限 Agent 形态「以 `SPEC-assistant-agent-mode`、`SPEC-agent-memory`
   与 `SPEC-agent-watch` 为准」，把语义权威倒挂。后续可以只改规范、不改意图。

无法验收或未闭合：

4. `ASST-002` 使用「状态有效的评论」，`CORE` 只定义帖子状态，没有评论的删除、作者、
   可见性与是否可编辑。
5. `REL-020` / `REL-054` 未覆盖记忆、Watch 任务/命中、`agent_run`；`MEM-004`
   衰减下限、「话题」、关键词匹配都不可观察；`MEM-012` 与 `REL-023` 对不齐；
   `MEM-022` 允许 403，和 `CORE-016` 的统一不存在冲突；记忆与 Watch 没有条数上限。
6. Agent 推荐未继承 `DISC-034` / `DISC-035`，卡片未要求 `DISC-032` 归因字段；
   助手推荐反馈不在 `REL-001` 事件集合中。
7. `WCH-020` 只注入「下一次 Agent 对话」，意图写「下次对话」。
8. `ASST-010` 成功事实回答 1～5 个来源，与意图中的「搜索综述」可能互斥。
9. `AGNT-A02` 仍只覆盖 Write + `search_posts` + `web_search`；授权披露、运行记录、
   附件没有对应验收。`REL-054-08` 与 `AGNT-061` 未分模式。

跨仓：前端 `INT-content-community-client` 停在 08-13，`SPEC-client-experience`
只补到 08-26 的 `FX-052`～`FX-058`，尚未承认评论证据、`consent_version`、记忆/Watch UI
与未知事件忽略。

---

# 建议变更

下列条款文本是建议稿，不是已批准规范。每项附推荐选项；需要人类拍板的标为「待决定」。

## 1. 拆 Agent SLO（`SPEC-feedback-reliability`）

待决定。推荐 **选项 A**。

- **选项 A（推荐）**：`REL-033` 将 Assistant 拆成两行。`enhanced_search` 保持
  月可用性 99.0%、首个事件 p95 2 秒、完成 p95 12 秒。Agent 模式：月可用性 99.0%、
  首个事件 p95 2 秒、完成 p95 45 秒。`REL-032` 增补：等待 `AGNT-020` 确认的时间
  不计入完成延迟；确认超时按协议成功结束，不计不可用。
- **选项 B**：Agent 完成不设 p95 时延目标，只保留可用性与首个事件；时延改观测。
- **选项 C**：维持 12 秒，把 `AGNT-030` 硬限降到能进 12 秒的步数。不推荐：与 08-27
  多工具综述能力冲突。

`REL-054-08` 限定为 `enhanced_search`（有证据则摘要降级）。Agent 模式 LLM 不可用
继续走 `AGNT-061`。矩阵新增行见第 4 项。

## 2. 收口网页来源（`SPEC-grounded-assistant` + `SPEC-assistant-agent-mode`）

待决定。推荐 **选项 A**。

- **选项 A（推荐）**：重批 `ASST-042`，增加来源类型 `web`，仅 Agent 模式可对外返回。
  关于社区内容的事实段落仍必须有 `[post:<id>]` 或 `[comment:<id>]`。
  网页引用必须标记为外部来源，不得写成平台事实或社区事实。旧客户端不得把未知
  来源解释为帖子证据（已有要求保持）。`web_search` 未配置时从工具表剔除（设计已如此，
  写入 `AGNT-012`）。
- **选项 B**：`web_search` 只允许作为写帖研究素材，禁止进入回答来源列表；写帖正文
  仍是用户将发布的内容，不适用 `ASST-010`。
- **选项 C**：从当前披露版本的白名单移除 `web_search`。不推荐：破坏 `consent_version`
  1 已覆盖工具。

无论哪一选项，禁止再以 `AGNT-*`「延伸」`ASST-042` 而不改 `ASST-042` 正文。

## 3. 意图去掉对规范的反向权威引用（`INT-content-community-backend`）

建议把非目标中「受限的 Agent 形态以 SPEC-… 为准」改为产品语言，例如：

> 不受用户授权、工具白名单和执行预算约束的通用外部网页问答或自主平台操作。
> 未批准新的意图修订前，不得对外承诺游戏库、价格、商城、通用通知中心，或超出
> 内容域搜索综述、结构化记忆、可解释推荐、条件追踪与本人帖子写操作的 Agent 能力。

规范继续把该边界写成可观察条款；意图不再点名下游文档。

`WCH-020` 与意图对齐，待决定：

- **选项 A（推荐）**：未读 Watch 摘要注入该用户下一次 **任一** Assistant 对话
  （`enhanced_search` 或 `agent`）。意图改为「下次 Assistant 对话上下文」。
- **选项 B**：维持仅 Agent 对话注入；意图改为「下次 Agent 对话」。

## 4. 评论对象模型（`SPEC-community-core`）

在 `CORE-022` 之后增加评论生命周期，不替代
[PROP-20260822](PROP-20260822-comment-reply-thread.md) 的楼中楼读取契约。

建议稿：

- `CORE-025`：评论由已认证用户创建，附着在当前可互动的已发布帖子上；创建后状态为
  `active`。作者可以删除自己的评论，删除后为 `deleted`（终态）。其它用户删除评论
  不属于当前范围。
- `CORE-026`：`active` 评论对能看到其父帖的访问者可见。`deleted` 评论对非作者统一
  表现为不存在，不得作为普通读取条目、发现结果或 Assistant 证据。
- `CORE-027`：评论没有独立 `revision`；作为证据时使用评论稳定标识 + 父帖 `revision`
  或内容 hash（与已批准 `ASST-011` 一致）。

`ASST-002` 的「状态有效」定义为 `active` 且父帖对当前用户为可见已发布帖子。
楼中楼是否对外读取仍等 PROP-20260822 人类决定；在该提案关闭前，规范不承诺回复列表。

## 5. 记忆 / Watch 的保留、降级与可观察匹配

### `SPEC-feedback-reliability`

`REL-020` 增补（推荐值，待决定）：

| 数据 | 保留 |
| --- | --- |
| 记忆当前记录（Profile / Interest / Task） | 直到用户删除；Interest 另受衰减；关闭个性化后 `behavior` 来源见 `MEM-012` |
| Episodic 与记忆冲突历史 | 90 天 |
| Watch 任务 | 直到用户删除或停用后 90 天 |
| Watch 命中与匹配执行记录 | 90 天 |
| Agent 运行记录与工具审计 | 30 天（与业务日志对齐） |
| 授权/撤销审计 | 90 天 |

`REL-054` 增补：

| ID | 故障 | 规定行为 |
| --- | --- | --- |
| REL-054-11 | 记忆存储不可用 | 对话可继续，不得谎报写入成功；Memory 工具失败并反馈模型；列表返回 503（`MEM-040`） |
| REL-054-12 | Watch 存储或匹配器不可用 | 创建/列表返回 503；已接受任务恢复前可漏匹配，恢复后不得把过期不可见内容补成新命中（`WCH-040`） |
| REL-054-13 | Agent 授权存储不可用 | Agent 请求失败关闭，不得当作已授权 |

### `SPEC-agent-memory`

- `MEM-004`：对外读取时 `score * exp(-λ Δt)`；λ 与下限可配置，但必须有已记录默认值。
  推荐默认下限 `0.20`，允许区间 `[0.05, 0.50]`。低于下限的 Interest 不得作为约束。
- `MEM-012`：关闭个性化后，来源 `behavior` 的当前记忆在 24 小时内停止作为推荐/检索
  约束与在线特征（与 `REL-023` 同一时限）；记录改为停用而非删除，用户仍可在记忆界面
  看到并手动删除。`explicit` / `conversation` 不变。
- `MEM-022`：越权读取/修改与 `CORE-016` 一致，统一返回不存在，不得用 403 泄露存在性。
- `MEM-033`（新）：同一用户当前 Profile+Interest 合计最多 200 条，Task 最多 50 条，
  Episodic 在保留期内最多 500 条；超额写入失败并反馈，不得静默丢弃最旧记录。

### `SPEC-agent-watch`

- `WCH-002` `keyword_new_post`：关键词 1～32 个 Unicode 字符；在新发布帖子标题或正文上
  做 Unicode casefold 后的子串匹配。空关键词拒绝创建。
- `WCH-003`：删除「话题」。`discussion_spike` 目标只能是帖子或标签。预筛选阈值必须可配置
  且可观测；推荐默认：24 小时窗口内评论数增量 ≥ 10。未达阈值不得调用模型。
- `WCH-006`（新）：同一用户同时启用的 Watch 任务最多 50 条；超额创建返回明确错误。
- `WCH-031`：命中保留 90 天（与上表一致），到期自动删除。

## 6. Agent 推荐接上发现与行为闭环

- `AGNT-018` 增补：`recommend_posts` / `similar_posts` 的候选必须遵守 `DISC-001`
  可见性、`DISC-034` 作者配额、`DISC-035` 隐藏/不喜欢排除。工具响应对每个条目携带
  `requestId`、位置、召回来源、规则或模型版本、实验标识（`DISC-032`）。
- `compare_posts` 只比较用户点名或本轮工具已返回、并已回源的标识；解释仍须区分平台
  可验证事实与推荐理由。
- 新条款 `AGNT-024`：Agent 推荐卡片上的曝光、点击、停留走既有
  `/api/v2/behavior/events`，`scene=agent`，动作集合不扩大 `REL-001`。
- 新条款 `AGNT-025`：`POST /assistant/recommend/feedback`（若保留）只写入助手作用域
  的推荐反馈并供记忆抽取，不是 `REL-001` 客户端行为事件，不得替代 hide/dislike。
  若不需要该接口，则从设计中删除，本条不写入规范。

待决定：是否保留独立的助手推荐反馈接口。推荐 **保留**，并按 `AGNT-025` 与行为事件划清。

## 7. 综述来源数与验收补全

- `ASST-010` 保持「每个事实段落至少一处社区证据」。
- 新 `ASST-016`：`enhanced_search` 整次成功回答社区来源仍为 1～5 个。Agent 模式
  搜索综述整次回答社区来源最多 10 个；网页来源另计，最多 5 个（若第 2 项选 A）。
- `ASST-A01` 增补评论证据成功路径。
- `AGNT-A02` 保持 v1 工具覆盖；新增 `AGNT-A09`：授权说明包含记忆与 Watch 披露、
  附件校验失败、`AGNT-053` 运行记录可查询且不含用户输入/回答全文。
- Memory / Watch 验收仍以 `MEM-A*` / `WCH-A*` 为准，不合并进 `AGNT-A02`。

## 8. 跨仓跟进（本仓不改前端文件）

人类接受本提案相关项后，前端另开提案或授权修订：

- `INT-content-community-client`：承认受限 Agent（授权、白名单、预算），非目标与后端
  对齐（不做游戏库/通知中心；网页检索不是通用问答）。
- `SPEC-client-experience`：评论来源可点击（`comment`）；`consent_version` 低于披露
  版本时重新展示完整清单；记忆列表/编辑；Watch 任务与命中；卡片与动作；忽略未知事件
  类型；授权文案覆盖记忆与追踪。
- `FX-051` 不得继续写成「只接受帖子证据」。

---

# 理由

- 第 1、2 项不处理，Agent 扩张在规格上不能闭合：实现无论怎么做都会违反其中一条
  `approved` 条款。
- 第 3 项恢复「意图 → 规范」单向权威，避免规范扩张默默改产品承诺。
- 第 4 项是评论证据的前置条件；楼中楼是独立产品决定，不绑在本次收口上。
- 第 5、6 项把 08-27 新状态纳入已有保留期、降级矩阵和发现/行为契约，避免第二套
  反馈闭环。
- 第 7 项让「综述」可验收，而不放宽 `enhanced_search` 的短回答来源上限。
- 第 8 项防止前后端规格各管各的：后端已批准的来源类型和授权披露，客户端按现行
  `FX-051` / `FX-053` 必须丢掉或少披露。

---

# 需要人类决定的事项

请按项选择或改写。未勾选的项保持 `approved` 原文，agent 不得自行落正式文档。

1. SLO：A 拆行（Agent 完成 45 秒、确认等待不计） / B 完成不设时延 / C 压缩工具步数 / 其他。
2. 网页来源：A 批准 `web` 类型 / B 仅写帖素材 / C 移出白名单 / 其他。
3. Watch 注入：A 下次任一 Assistant 对话 / B 仅下次 Agent 对话。
4. `REL-020` 保留期与记忆/Watch 条数上限：接受建议表，或给出替代数字。
5. Interest 衰减默认下限 `0.20`、`discussion_spike` 默认「24h 评论增量 ≥ 10」、
   关键词 casefold 子串：接受或改阈值。
6. 是否保留 `POST /assistant/recommend/feedback`。
7. 评论 `CORE-025`～`027` 是否现在写入；楼中楼是否仍走 PROP-20260822 单独批。
8. 前端规格是否在本轮后端修订批准后立即跟进。

---

# 影响

- 接受后：获授权 agent 才能改对应 `INT-*` / `SPEC-*`，并同步规范层 README 修订日志、
  设计与实现追踪。未接受的项继续 `diverged` / `partial`，不得用设计消掉。
- 不自动关闭 PROP-20260822、PROP-20260813-slo-synthetic-observation。
- 不授权改代码。SLO 数字、来源类型、注入范围未定前，实现应继续按现有设计推进并在
  IMP 标明偏离。
- `TRANSITION.md` 仍写「设计尚未引用新规范」，与 `DES-assistant-agent-runtime` 已
  引用新规范不符；那是过渡页过期，agent 可在实现层维护中修正，不依赖本提案批准。
