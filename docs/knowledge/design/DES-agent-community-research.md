---
id: DES-agent-community-research
layer: design
title: 社区复杂需求与可信回答交付
status: active
owner: agent
upstream:
  - SPEC-assistant-agent
  - SPEC-agent-memory
  - SPEC-agent-watch
  - SPEC-content-discovery
  - SPEC-feedback-reliability
---

# 社区复杂需求与可信回答交付

## 分层迁移与不变机制

规格只保留工程约束、质量指标和验收条件。MySQL 权威状态、Redis 辅助通知、ES 派生历史、独立 worker、
60 秒租约/10 秒续租、command journal、CAS 确认、50% compact/20% 保留等既有实现选择仍由
[基础运行时设计](DES-assistant-agent-runtime.md)承接。本次不更换这些组件、不修改资源或隐私阈值，
也不将文档整理视为现有全部实现已经对齐。

## 协议与工具

消息请求增加可选 `clientProtocolVersion`，缺省 1，新客户端为 2，接受后随 run 持久化。能力快照仍在
冷启动、30 分钟冷对话拼接或 compact 时生成；旧快照不被强制重写。模型广告按快照、任务来源、授权
和冻结的客户端协议取交集。新协议提供 `ask_questions`、`read_source`、`publish_answer`；旧协议继续
原有 `present_sources` 路径。Watch 可使用读取/发布，但没有问答与新增平台写权限。

工具仍由统一 registry metadata 派生 schema、任务来源、授权、可用性和结果限额。新工具不扩大个人
数据访问权限，也不引入 Intent Router。系统规则和 SOUL 明确社区优先、按需澄清、互联网补充、诚实
说明不足和逐项引用。模型行为的语义质量必须通过真实轨迹和人类冻结集核验，结构验证不能证明语义。

## 问答状态机

```text
model/tool step -> ask_questions -> waiting_input
  -> 真实答案 -> queued -> 新租约接管 -> 对应 tool result -> 后续模型轮
  -> 新普通消息 -> superseded -> 补充/转向同一 run
  -> Stop/撤权 -> cancelled
  -> 到期 -> expired + AGENT_RESOURCE_LIMIT
```

每批 1～3 个问题，每问 2～8 个单选/多选选项；问题及选项有稳定 ID，问题文字最多 300 字、选项 200 字。
用户补充文字合计最多 2,000 字。每题必须明确为 answered、unknown、no_preference 或 skipped；未知和
跳过不得携带选中项，选项不能串题或重复。先搜索只将用户尚未回答的题明确记为跳过。

`agent_question_request` 绑定 run/user/call/message，保存问题、答案、期限和提交幂等信息。问题记录、
可见问题消息及 questions_required 事件在 run step 事务中一起提交，再将 run 置为 waiting_input。
worker 随即释放，claim 不再领取等待任务；该任务仍占用本人前台位置，其他用户不被等待阻塞。
可见问题仅为 UI 投影，不重复加入 provider 历史；原始工具调用及真实结果仍按原字节恢复。

新增 `POST /api/v2/assistant/runs/:id/answers`，输入 questionRequestId、requestId、逐题答案。
事务先锁 run，复核用户、授权、状态和期限，再条件接受一次并持久化工具结果、questions_resolved 与
queued 状态。相同提交返回原结果，不同提交竞争返回冲突。Stop、撤权、普通消息和到期遵守同一锁序。

等待截止时间为 `min(等待开始+30分钟, run开始+6小时)`。独立扫描器每秒有界检查等待任务，网络心跳
不刷新活动时间。到期不执行默认答案。用户显式继续通过 messages 的 questionContext 引用旧问题和
真实答案，服务端重新校验归属与历史可见性并创建新 run，旧终态不可重开。

## 检索与证据

现有搜索、推荐、帖子和评论读取将实际片段登记为 `agent_source_evidence`，ID 由任务内来源与实际
内容确定；记录片段类型、评论身份及取得时间。工具返回片段与 evidenceId，不只返回网页标题。
`read_source(handle,cursor)` 只能读取当前 run 的来源，帖子按 1,200 字窗口回源并检查原 revision；
网页只使用搜索服务实际返回的摘录，不新增任意 URL 爬虫，不声称已取得网页全文。

发布时帖子重新验证归属边界、published 和 revision；评论通过 Content RPC 的 GetCommentsByIds 按
父帖批量回源并校验状态及实际片段。网页 URL 仅允许无凭证的 HTTP/HTTPS 公共地址，拒绝本机、私网和
危险 scheme。来源存在性与内容支持关系是不同证据：后者仍需人类语义评审。

## 回答原子发布

`publish_answer` 接受有序 blocks，每段有 kind、text 和 citations。fact、experience、inference 必须
引用实际 handle/evidenceId；context 仅承载用户自述或条件，limitation 仅说明缺口。最多 10 篇来源，
同一来源合并为一张卡，多个证据片段保留各自身份。正文经过现有输出清理，不从自由文本推断可信链接。

取得来源后模型普通正文不直接对外流出；过程只展示工具进度。若最终返回绕过结构化发布的正文，明确
失败而不宣称检索回答成功。结构错误返回工具结果供修正，重复无进展仍受统一 guard 和原有预算约束。

完成事务原子写入工具结果、assistant_message、assistant_message_presentation、索引 outbox、线程摘要
和 answer_committed/done。正文和引用编号由同一 presentation 生成，不使用字符偏移猜测对应关系。
Watch 使用同一发布机制，并保持命中复核、配额完成和未读更新的原子边界。

## 读取、兼容与删除

线程提供当前待答问题；历史和 SSE 提供类型化 questionRequest/answerPresentation，使用同一持久快照。
来源再读取时复核可见性；失效卡隐藏标题、图片和摘录，标明受影响引用。正文保留历史记录性质，不再
当作当前已核实依据。新字段为增量扩展，旧字段、未知事件忽略和原有流式尝试隔离保持。

新表与客户端协议列通过幂等增量 patch 创建，不触发旧清库迁移。问题和展示快照随消息物理清理，
来源片段遵循 365 天期限；清除历史取消待答状态并清理来源与展示数据，不删除 MEMORY/USER/Watch。
当前源码、实际测试及剩余边界由实现映射和带日期证据记录。
