---
id: SPEC-agent-watch
layer: spec
title: Agent Watch 主动私信规范
status: approved
owner: human
upstream:
  - INT-content-community-backend
updated_at: 2026-09-05
---

# Agent Watch 主动私信规范

## 范围

Watch 是用户委托 Agent 持续关注作者、标签、关键词、帖子修订或讨论量的任务。条件由内容/行为事件
匹配，命中后调度受限只读 Watch run，最终通过“小白盒 Agent”虚拟线程主动发消息。Watch 不是普通
私信、点赞/评论/关注通知中心或推送产品。

## 任务与匹配

- `WCH-001`：任务绑定当前用户，包含稳定 id、条件类型、目标、启用状态和版本。用户可以列表、创建、
  修改、停用和删除自己的任务；同一用户、条件与目标只允许一条启用任务。
- `WCH-002`：规则条件为 `author_new_post`、`tag_new_post`、`keyword_new_post`、`post_revised`；
  `discussion_spike` 必须先由行为/计数字段过可配置阈值，再允许模型判定。禁止定时空调用模型。
- `WCH-003`：未发布、取消发布、删除或用户不可见的帖子不得产生可投递命中；目标不存在时任务创建
  失败。匹配失败不得影响内容、评论或行为主路径。
- `WCH-004`：匹配和执行可去重、可审计，能确认是否经过预筛选、是否调用模型及结果。内部执行记录
  保留 90 天，只用于合并、去重和审计，不作为用户可见收件箱；数据结构归设计。

## 合并、优先级与主动消息

- `WCH-010`：同一用户两分钟内的命中合并成一次 Watch run；同一任务每小时最多主动发送 3 条、
  每用户每天最多 20 条，超额命中进入下一次允许发送的摘要，不丢失审计记录。
- `WCH-011`：Watch run 只能调用搜索、回源、推荐、读取 MEMORY/USER、`search_history` 和
  `present_sources`；禁止帖子、Memory、Watch 与其它平台写操作。
- `WCH-012`：Watch run 优先级低于用户 run。用户输入抢占 Watch 时，worker 安全停止并把未发送命中
  重新合并，不能留下半条主动消息或把 run 标成成功。
- `WCH-013`：成功 Watch run 向 Assistant session 写一条 assistant 消息，计入线程未读并触发消息
  徽标；不创建机器人用户或普通 message conversation，也不写普通通知中心。
- `WCH-014`：命中本身不是可信来源，展示前须回源。主动消息同样使用校验后的回答和来源快照，
  每条实质性信息关联具体帖子或网页并展示相关原文摘要，遵守 `AGENT-073`~`AGENT-075`。

## API、保留与失败

- `WCH-020`：保留 Watch task CRUD；删除用户可见 Watch hits/list/read API。用户从 Assistant 线程读取
  主动消息及未读状态，不再维护独立命中收件箱。
- `WCH-021`：任务创建、修改与删除不走 `delete_post` 逐次确认，但必须通过 Agent consent、归属、
  version、参数校验、幂等和审计。
- `WCH-022`：Watch matcher 或 worker 不可用时，任务 CRUD 可独立报告依赖状态；恢复后只能处理仍在
  保留期且当前可见的命中，不能把过期或不可见内容补成新消息。
- `WCH-023`：清除 Assistant 历史或冷对话拼接不删除 Watch task；撤销 Agent consent 停止新 run，
  但任务保留为禁用前状态供重新授权后继续。

## 验收标准

- `WCH-A01`：覆盖四种规则条件、discussion_spike 预筛选、不可见目标不命中和事件去重。
- `WCH-A02`：覆盖两分钟合并、每小时/每日上限、超额摘要以及 90 天内部保留。
- `WCH-A03`：覆盖 Watch 只读工具表、用户 run 抢占并重新合并，以及禁止任何平台写操作。
- `WCH-A04`：覆盖主动消息进入虚拟线程并计未读、不创建普通私信、旧 hits API 不存在。
- `WCH-A05`：覆盖 task CRUD 越权、版本冲突、删除后不再匹配和恢复时不可见内容不补投。
