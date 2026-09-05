---
id: SPEC-assistant-agent
layer: spec
title: 持久异步 Assistant Agent 规范
status: approved
owner: human
upstream:
  - INT-content-community-backend
updated_at: 2026-09-05
---

# 持久异步 Assistant Agent 规范

## 范围与身份

本规范统一约束帮助用户使用内容社区的“小白盒 Agent”：复杂需求澄清、社区优先检索、互联网补充、
逐项 URL 引用，以及消息虚拟线程、异步运行、平台工具、历史召回、授权、预算和恢复语义。
它接替已 retired/deprecated 的 `SPEC-grounded-assistant` 与
`SPEC-assistant-agent-mode`。Memory 与 Watch 的专门行为分别由 `SPEC-agent-memory` 和
`SPEC-agent-watch` 约束。

- `AGENT-001`：Assistant 是每个认证用户拥有的一条虚拟私信线程，不创建机器人用户，不写入普通
  私信 conversation/message 模型，也不改变普通 Message API。
- `AGENT-002`：Assistant 使用统一的对话与工具执行行为，不存在 enhanced_search/agent 模式开关；
  `/api/v2/assistant/chat` 与请求 `mode` 字段硬删除，不提供兼容适配层。
- `AGENT-003`：Agent 可以直接对话，也可以根据 system prompt、SOUL、冻结的 MEMORY/USER、当前
  会话和工具 schema 自主选择工具及参数；服务端不得先用关键词或小模型对话轮做 Intent Router。
- `AGENT-004`：用户 run 以当前认证用户身份执行，只能访问该用户直接可访问的数据，只能更新或
  删除本人帖子；Watch run 只读，memory-review run 只能读写 MEMORY/USER。

## 复杂需求与社区优先

- `AGENT-100`：Agent 是内容社区的辅助工具，重点解决需要澄清条件、多轮检索、比较和综合的复杂
  自然语言需求。普通对话仍可直接回答，但不以独立通用聊天或长期陪伴产品作为建设目标，不替代
  普通社区搜索、推荐和内容操作入口。
- `AGENT-101`：需要资料支持的复杂需求先明确搜索条件，再优先检索可访问的社区帖子与讨论；按需要
  拆分问题、调整查询和综合多份资料，不能把一次关键词命中列表当作完整解答。条件已知或用户按
  `AGENT-113` 要求先搜索时，不强制先调用澄清工具。
- `AGENT-102`：社区无相关资料、资料不完整或不足以支持判断时，必须尝试使用可用且已授权的互联网
  搜索引擎补充，并向用户说明当前社区不能解决的全部或具体部分；部分可解时保留社区依据，不把
  外部资料伪装成社区结论。网络搜索不可用时说明能力限制，不得声称已经搜索。
- `AGENT-103`：检索失败不等于没有内容；社区搜索故障、可见性无法验证与资料不足必须区分，不能把
  故障归因为社区缺少资料。仍遵守可见性失败关闭、工具授权和资源预算；外部资料也不足、不可核实
  或相互矛盾时，提供有依据的部分回答并说明限制，不能为了完成任务编造信息或给出无依据的确定结论。
- `AGENT-104`：最终回答用自然语言解释资料、比较取舍并回应用户条件，不只列出链接。每条来自资料
  的实质性信息按 `AGENT-073`~`AGENT-075` 引用；区分来源陈述、个人体验与综合判断，不能把单个体验
  或热度当作普遍结论。评价理解条件、材料相关性、引用支持关系与任务解决程度，而非聊天时长。

## ask_questions 需求澄清

- `AGENT-110`：提供 `ask_questions` 结构化工具，由 Agent 根据当前需求自主调用，不新增关键词
  Intent Router。问题和选项应能由客户端展示为可操作的问答；用户提交的答案作为对应工具结果参与
  后续判断，不以在普通正文列出问题代替结构化工具。
- `AGENT-111`：优先使用单选或多选，提供问题、可理解的选项及选择类型；允许文字补充，并在适用时
  提供“不知道/没有偏好”或跳过。选项不能覆盖用户情况时不得强迫其选择不准确的答案。
- `AGENT-112`：分轮只询问会显著改变搜索方向或判断的关键信息，复用当前对话已明确的条件。搜索条件
  明确是指能够描述目标、适用情况、硬约束、比较重点与仍未知的条件；足以有效检索时停止追问，不以
  填满预设问卷为门槛，也不重复询问已经回答的信息。
- `AGENT-113`：用户选择未知、没有偏好、跳过，或明确表示“先帮我看看”时，允许在未知条件下先搜索。
  不得补造用户答案、将跳过解释为某个默认选项，或把推测写成用户已确认的偏好；未知条件影响结论时
  明确适用前提，给出分情况建议或说明尚不能判断的部分。
- `AGENT-114`：用户回答后重新判断关键缺口，必要时继续追问；检索发现新的关键分歧时也可再次调用。
  不重复追问用户已明确跳过的问题，除非先解释新的必要性，且用户仍可跳过。等待回答不能被解释为
  用户已经选择、同意或确认。
- `AGENT-115`：`ask_questions` 只用于用户 run 的需求澄清，答案须关联正确用户和对应问题；不能作为
  capability consent 或 `delete_post` 确认，也不能绕过现有工具权限。Watch 与 memory-review 不调用
  该交互工具，保持各自现有权限边界。

## 线程、会话与消息

- `AGENT-010`：`GET /api/v2/assistant/thread` 返回虚拟线程摘要、当前 session、未读数和活跃 run；
  `GET /api/v2/assistant/messages` 只返回该用户的 Assistant 消息。
- `AGENT-011`：`POST /api/v2/assistant/messages` 先持久化用户消息与 run，再返回 `messageId`、
  `sessionId`、`runId` 和 `disposition=started|redirected|steered|queued`，不等待模型完成。
- `AGENT-012`：每用户同时最多一个前台 run。新消息在模型请求阶段可安全 redirect，在工具阶段
  steer；compact、附件处理或其它不能安全注入的阶段进入最多 32 条 FIFO，超限明确拒绝。
- `AGENT-013`：每用户仅一条永久前台 session。`POST /api/v2/assistant/sessions` 硬删除，不提供
  兼容适配层。线程上一条可见消息距今不少于 30 分钟后，下一次新建 user 或 Watch run 时在同一
  session 上滚动 prompt epoch，并按字节重建安全规则、`SOUL.md`、工具规则与冻结 MEMORY/USER；
  不删除历史消息、不把历史移出 live prompt、不删除 MEMORY/USER 或 Watch。redirect、steer、FIFO
  与崩溃恢复不得因空闲拼接而重写快照。`DELETE /api/v2/assistant/history` 删除 Assistant 历史，
  不影响 MEMORY/USER 或 Watch。
- `AGENT-014`：`POST /api/v2/assistant/thread/read` 更新 Assistant 未读；主动 Watch 消息计入未读，
  `memory_changed` 系统行不计未读。
- `AGENT-015`：消息正文 `content` 与真实 provider-bound `api_content` 分离保存。`content` 仅用于
  用户可见历史；恢复 run 必须重放原始 `api_content` 字节。冻结 MEMORY/USER、BM25、Watch 与其它
  平台上下文只能进入结构化 sidecar，不能反写到可见正文或在恢复时重新拼接成不同字节。

## 异步运行与恢复

- `AGENT-020`：`assistant-rpc` 负责 API/read model；独立 `assistant-agent` worker 执行 run；
  Watch matcher 负责匹配与调度，不在请求进程同步调用模型。
- `AGENT-021`：MySQL 是 run、message、event、tool call 与 prompt 快照的权威库。worker 通过数据库
  lease queue 取任务；租约 60 秒，每 10 秒续租，崩溃或租约失效后从最后已提交 step 恢复。
- `AGENT-022`：客户端断线不取消 run。只有显式 Stop 或取消 API 才请求硬取消；用户前台 run 可
  抢占 Watch 与 memory-review，未发送 Watch 命中重新进入合并窗口。
- `AGENT-023`：SSE 事件先以单调 `seq` 写 MySQL，再尝试 Redis 通知。`Last-Event-ID` 后的事件必须
  补齐；Redis 不可用时以 MySQL 轮询降级，且不得丢失终止事件。
- `AGENT-024`：`GET /api/v2/assistant/runs/:id/events` 只允许 run 所属用户读取；持久事件类型为
  `run_started|token|response_reset|tool_call|tool_result|confirm_required|source_card|memory_changed|done|error`。
  `token` 与 `response_reset` 必须携带同一 model attempt 的稳定 `streamId`；旧客户端仍按未知事件忽略
  `response_reset`，不得因此中断 SSE。
- `AGENT-025`：run 只有一个终止状态。错误终止保留已经提交的部分文本、已完成副作用摘要和完整
  事件序列，不得先完成后报错或伪造成功。
- `AGENT-026`：每次 provider stream writer 绑定
  `(run_id, lease_generation, input_version, model_round, attempt)`。新 attempt、lease 接管或输入 redirect
  必须 fence 掉旧 writer；若旧 attempt 已产生持久 token，重试前先提交 `response_reset`，确保事件重放
  只组装最终获胜 attempt。token 允许按时间或字节批量提交，不得逐 token 热写 MySQL。

## 授权、工具和副作用

- `AGENT-030`：用户必须显式授予 Agent capability consent；授权说明列出当前工具分组、数据边界、
  delete_post 逐次确认、Memory/Watch 和长任务预算。撤销后不接受新用户 run，活跃 run 安全停止。
- `AGENT-031`：用户 run 可获得授权版本覆盖的完整工具集；Watch run 只允许搜索、回源、推荐、读取
  MEMORY/USER、`search_history` 与 `present_sources`；memory-review 只允许 Memory 工具。
- `AGENT-032`：只有 `delete_post` 逐次确认。create/update、Memory 与 Watch 写仍须通过授权版本、
  schema、所有权、revision、幂等和审计校验，但不弹逐次确认。
- `AGENT-033`：工具副作用写入 command journal，以
  `(user_id, request_id, tool, canonical_args_digest)` 唯一。恢复或重复调用返回已提交结果，不能再次
  执行成功动作；参数摘要使用规范化 JSON 后计算。
- `AGENT-034`：删除确认一次性绑定 user/session/run/call/工具/规范化参数摘要/目标 revision；
  服务端用数据库 CAS 裁决。跨 run、参数变化、revision 变化、重复确认或过期确认一律无效。
- `AGENT-035`：工具输入和工具返回均是不可信数据，不得改变系统安全规则、可用工具、归属校验、
  确认或预算。平台不得提供账户 secret、验证码、普通私信、其它用户记忆或未发布内容工具。
- `AGENT-036`：每个工具由单一 metadata 声明 schema、effect=read|write、允许 run source、最低 consent、
  confirmation、availability、幂等类型和最大结果大小；模型广告、执行授权、journal、确认与结果上限
  必须由该 metadata 派生，不能在 runtime 另设副作用白名单。依赖不可用的工具不得进入新 prompt epoch。
- `AGENT-037`：工具参数按声明 schema 严格解码；未知字段、尾随第二个 JSON 值、类型错误和越界值必须
  拒绝。所有 run 对重复失败或无进展调用执行统一 guard：按工具、规范化参数和规范化结果/错误识别，
  第二次给模型不可见收敛提示，第三次仍无进展时以 `TOOL_NO_PROGRESS` 明确终止；轮询型工具可显式豁免。

## 模型传输与 Prompt

- `AGENT-040`：同时支持 Responses 与 Chat Completions。每个 provider/model 以声明式 capability profile
  记录 route、WireAPI、工具与流式支持、上下文窗口和输出上限；启用 LLM 时启动阶段使用强制无副作用
  工具调用 canary 验证 schema/call/result，失败则 readiness false，不能静默降级成无工具模型。
- `AGENT-041`：Prompt 顺序固定为：不可覆盖的平台安全规则 → 仓库版本化 `SOUL.md` → Agent/tool
  规则 → 结构化且明确标为不可信数据的冻结 MEMORY/USER sidecar → 当前会话历史。MEMORY/USER 不得作为
  system 指令或 authoritative source，sidecar 标签和内部说明不得出现在用户可见输出。
- `AGENT-042`：`SOUL.md` 为 human-owned 仓库资产，用户不能编辑；默认表达温暖、诚实、克制，服务于
  社区辅助工具定位，不以独立陪伴为目标。新 SOUL
  仅在冷对话拼接、无快照冷启动或 compact 成功提交的新 prompt epoch 生效。
- `AGENT-043`：system prompt、Memory sidecar、工具定义和 provider capability 在冷对话拼接或无快照
  冷启动时构建并按字节保存；恢复未 compact session 必须复用原始快照，不受仓库、Memory、工具表
  或默认 provider 随后变化影响。实时撤权仍直接取消 run，不能靠修改已冻结快照表达。
- `AGENT-044`：provider 错误统一分类为 auth、invalid_request、context_overflow、rate_limit、timeout、
  overloaded、server_error、content_policy 或 unknown，并携带状态码、可重试性与有界 `Retry-After`。
  rate limit/timeout/5xx 最多重试三次并使用有上限的抖动退避；确定性 4xx 不重试。fallback 只允许切到
  工具、流式、上下文、数据地域和隐私边界兼容的已配置 route，每个 attempt 必须持久审计。
- `AGENT-045`：Responses、Chat Completions 与兼容网关 usage 统一规范化 input、output、cache-read、
  cache-write 与 reasoning token；总 input 不得与 cache bucket 重复累计，成本使用各自配置价格。provider
  缺失 usage 时明确标为估算，不能把 cache 命中默认为零后声称 cache 观测有效。

## Compact 与后台审查

- `AGENT-050`：以上一次 provider 真实 prompt usage 为锚点并估算其后新增内容；无 usage 时使用对 CJK
  至少按一字符一 token 的保守估算。达到模型窗口 50% 时 compact，保留最近 20% token、未完成工具
  调用与确认，并加入压缩摘要。摘要输入按总预算选择完整消息，不能固定截断每条消息。只有压缩后
  token 确实下降且低于目标阈值才提交；成功后滚动 prompt epoch，重新加载 SOUL、MEMORY/USER、工具
  与 provider capability 快照。
- `AGENT-051`：compact 前原始消息不删除，继续保留一年并可被 `search_history` 召回；摘要不能覆盖
  权威消息，也不能把不可信历史提升为系统规则。
- `AGENT-052`：每 10 个成功且未中断的用户回合调度 memory-review run；最多 16 轮、累计输入最多
  600,000 token，仅允许 Memory 工具，不写主会话。新前台消息优先取消未完成的审查。
- `AGENT-053`：后台审查成功写入后增加不计未读的 `memory_changed` 系统行并提供撤销；审查失败不
  改判主会话成功状态。
- `AGENT-054`：memory-review 使用 `BackgroundReview.Model` 配置的同边界辅助 route，compact 优先使用
  `LLM.AuxModel`；未配置时使用冻结主 route。辅助调用继承超时、错误分类和 usage 审计，但不得改变
  前台 session 的冻结 provider capability。

## 历史 BM25

- `AGENT-060`：Elasticsearch 只建立 Assistant 历史的 CJK/BM25 派生索引，按 `userId` 强隔离；
  MySQL 始终权威。所有命中返回前回源校验 user、删除状态和 365 天保留期。
- `AGENT-061`：`search_history` 支持关键词发现、围绕锚点滚动、读取会话和浏览最近会话四种形态；
  默认 Top 3、最大 10。首结果包含锚点前后各 5 条及会话首尾摘要，低排名只含锚点。
- `AGENT-062`：默认只搜索 user/assistant 消息，排除 tool、memory-review 与当前 live context；允许
  已结束会话和 compact 掉的消息。普通用户之间私信永远不在索引或结果范围内。
- `AGENT-063`：索引通过 MySQL outbox 派生，支持 rebuild 与删除传播；ES 故障使 `search_history`
  工具明确失败，不影响 MySQL 中线程、消息和 run 的权威读写。

## 来源 Ledger 与逐项 URL 引用

- `AGENT-070`：搜索、推荐和 web 工具把经服务端验证的结果登记为本 run source ledger，并只向模型
  返回不可伪造、绑定 run 的 source handle。handle 不能跨 run 使用。
- `AGENT-071`：`present_sources` 接受本 run 至多 10 个有效 handle，生成 `source_card` 结构化事件；
  保留该展示能力，但不强制将所有引用渲染为卡片。普通闲聊不要求来源；复杂需求的检索回答必须满足
  逐项 URL 引用，不能以未调用 `present_sources` 或未展示卡片为由免除引用义务。
- `AGENT-072`：模型自行生成的帖子 ID、链接或引用不能成为可信来源；可信来源来自经服务端验证的
  结构化来源记录，不能从模型正文反推。已有 `source_card` 事件保留；正文引用也必须关联验证后的
  来源。过期、越权、删除或 revision 不匹配的 source 在展示前重新校验并剔除或标记不可用，不得
  继续把依赖失效来源的信息呈现为已经核实的结论。
- `AGENT-073`：复杂需求的检索回答中，每条实质性的事实、经验反馈、比较依据和建议依据都必须在
  对应句子、条目或段落附近关联引用；社区资料给出具体帖子 URL，外部资料给出具体网页 URL，不以
  网站首页、搜索结果页或文末无对应关系的链接列表代替。一个来源可支持多条信息，但关联必须明确。
- `AGENT-074`：引用必须来自实际检索到的资料，且资料确实支持旁边的表述；不得编造 URL、引用未取得
  的内容，或把不相关链接作为装饰。已知条件和用户自述须注明其性质，不伪装成站内外资料；普通闲聊、
  澄清问题与任务状态说明不因无外部引用而失败。
- `AGENT-075`：综合判断必须明确为 Agent 基于资料的推导，并引用支撑资料；多份资料共同支持时列出
  相应引用。资料相互冲突时分别引用并解释差异和适用条件；缺乏支持时说明未知、降低结论确定性或
  不作该判断，不能把猜测包装成来源原文。

## 资源预算与观测

- `AGENT-080`：每 run 硬上限为 500 模型轮、1,000 工具调用、30 分钟无活动、6 小时绝对时长、
  1,000,000 总生成 token；单次输出上限为 `min(provider_limit, 65,536)`。
- `AGENT-081`：warning 阈值为 5 分钟、30 轮或 100k 输出 token；critical 为 20 分钟、100 轮或
  500k 输出 token。每 run 每级每维只告警一次，仅进入 metrics、日志、Prometheus 和发给模型的
  不可见收敛提示，不向用户消息写预算文案。
- `AGENT-082`：真正触顶以 `AGENT_RESOURCE_LIMIT` 终止，并保留部分文本与已完成副作用摘要。
- `AGENT-083`：观测 run elapsed/idle、queue age、rounds、tool calls、input/output/cache tokens、cost、
  lease recovery、compact、BM25、Watch、memory-review 与 Redis 通知降级，且不得记录消息正文、
  prompt、secret 或普通私信。

## SLO 与验收

- `AGENT-090`：接收 p95 500ms、首持久事件 p95 2s、活跃心跳间隔不超过 30s；普通完成 p95 45s
  仅作观测目标，长任务不设统一完成 SLO；Watch 命中到主动私信 p95 5 分钟。
- `AGENT-A01`：覆盖 lease/recovery、一个前台 run、redirect/steer/FIFO、显式 Stop、断线续流、
  Redis 降级和终止唯一性。
- `AGENT-A02`：覆盖 command journal、删除确认 CAS、版本/参数变化、权限与工具分组。
- `AGENT-A03`：覆盖 50% compact、prompt 字节稳定、20% 保留、新 epoch 加载 SOUL/Memory、后台审查
  隔离和撤销。
- `AGENT-A04`：覆盖 BM25 用户隔离、四种历史调用、365 天回源、rebuild/delete 与私信永不入索引。
- `AGENT-A05`：覆盖 source handle run 绑定、`present_sources`、正文伪造来源无效、普通闲聊无需
  来源，以及检索回答无卡片时仍须提供逐项可信 URL 引用。
- `AGENT-A06`：覆盖两种 LLM transport 的工具调用，以及硬预算触顶与每维每级只告警一次。
- `AGENT-A07`：覆盖两种 WireAPI 的真实流式 fixture、跨 chunk tool call、attempt reset、lease/input
  fencing、断线重放，以及前端最终文本只包含获胜 attempt。
- `AGENT-A08`：覆盖 capability snapshot 恢复、启动 canary、typed retry/Retry-After/fallback、cache usage
  规范化、中文 compact 与压缩无收益拒绝提交。
- `AGENT-A09`：覆盖严格工具 schema、availability/output limit、普通 user/Watch/review 的 no-progress
  guard，以及 Memory sidecar 不能覆盖系统规则或泄漏到流式/非流式输出。
- `AGENT-A10`：以“选择适合的猫粮”“如何选择适合自己的狗”等复杂需求验证社区优先、多轮检索与
  比较综合；覆盖社区充分、部分不足、无资料、检索故障、外部补充成功与不可用，核验不足说明真实，
  不绕过可见性检查，也不以单次结果列表或空结果代替尽力解答。
- `AGENT-A11`：覆盖 `ask_questions` 单选、多选、文字补充、分轮追问、已知条件不重复询问、条件足够
  即检索，以及未知/无偏好/跳过/要求先搜索时的分情况回答；不补造偏好，不默认为授权或删除确认，
  不把答案关联到其他用户或问题，Watch/review 不调用该工具。
- `AGENT-A12`：覆盖回答后继续澄清与检索后出现新分歧；重问已跳过项须说明新必要性且仍可跳过，
  等待期间不得伪造工具答案。
- `AGENT-A13`：逐项核验自然语言回答的信息与帖子/网页 URL 对应、来源实际取得且支持表述；覆盖多源
  推导、冲突资料、个人体验与普遍事实区分、无依据判断、伪造/失效 URL、文末仅堆链接，以及普通
  闲聊和澄清不强制引用。既有人类冻结集要求不变，合成样例只验证流程，不证明实际回答质量。
