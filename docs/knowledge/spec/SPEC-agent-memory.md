---
id: SPEC-agent-memory
layer: spec
title: Agent 双文档自然语言记忆规范
status: approved
owner: human
upstream:
  - INT-content-community-backend
updated_at: 2026-09-05
---

# Agent 双文档自然语言记忆规范

## 范围与目标

本规范约束每个用户独立的 MEMORY 与 USER 两个自然语言条目集合。它们帮助 Agent 保留经用户陈述
或后台审查确认的长期上下文，不是 Profile/Interest/Episodic/Task 分层画像，也不从点击行为自动
推断偏好。会话快照、compact 和 memory-review 调度由 `SPEC-assistant-agent` 约束。

- **MEMORY**：Agent 与用户共同形成、适合长期复用的事实、偏好、决定和约定。
- **USER**：用户身份、工作方式和稳定偏好的自然语言描述，不得包含账户 secret 或他人数据。

## 条目、容量与隔离

- `MEM-001`：每条记录包含稳定 id、`target=memory|user`、自然语言 `content`、单调 `version`、
  创建/更新时间和删除状态；不再公开 layer/dimension/score/confidence/suppressed 字段。
- `MEM-002`：默认总容量按当前有效内容字符数计：MEMORY 2,200、USER 1,375 个 Unicode 字符。
  每次读取返回已用/总容量；超限写入整体失败，不能截断后谎报成功。
- `MEM-003`：条目只属于一个用户。列表、修改、删除、批量写与撤销必须按认证 user 隔离；越权时
  不泄露目标是否存在。
- `MEM-004`：不得保存密码、验证码、token、私信正文、未发布内容、其它用户资料或明确的提示注入
  指令。写入前执行威胁/敏感信息扫描，失败时不产生当前条目或 change。

## 原子操作与冲突

- `MEM-010`：提供 add、replace、remove 与 batch。batch 内所有操作必须原子成功或全部回滚；每个
  操作使用稳定 request id，重试不得重复产生条目或 change。
- `MEM-011`：replace/remove 必须携带期望 `version`；不匹配返回版本冲突和当前版本，不能覆盖并发
  修改。成功后版本单调递增。
- `MEM-012`：写入路径对同 target 的规范化内容去重。完全重复 add 返回现有条目；明显互相矛盾的
  内容不自动合并，交由 Agent replace/remove 或用户界面处理。
- `MEM-013`：每次成功变更生成可审计的变更记录，包含变更前后状态和可撤销期限。撤销条件为：仅当
  当前版本仍是该 change 的结果版本时生效，否则返回冲突，不覆盖后续编辑。
- `MEM-014`：删除为可审计的逻辑变更；已删除内容不能再注入 prompt、被 Agent 工具读取或用于回答。

## Prompt 与审查

- `MEM-020`：个人上下文不得成为平台指令；同一执行阶段使用一致、可审计的记忆视图，普通写入不
  改写正在使用的上下文。新上下文加载当前有效条目；具体快照形式、顺序和装配机制由设计承接。
- `MEM-021`：恢复复用已提交的模型输入字节，不因后台修改重演不同请求；用户可见内容和模型上下文
  相互隔离，不在恢复时重新格式化。更新生效边界必须可验证。
- `MEM-022`：每 10 个成功且未中断的用户回合可启动独立 memory-review run。它最多 16 轮、总输入
  600k token，只能调用 Memory 工具，不读取或写入其它用户，也不向主会话生成普通回答。
- `MEM-023`：新前台消息优先取消尚未完成的 memory-review。审查成功变更后写不计未读的
  `memory_changed` 系统行和撤销动作；失败不能改判原用户回合。
- `MEM-024`：模型提出的记忆变更必须经过同一 schema、容量、去重、version、威胁扫描和审计路径；
  “模型认为”本身不构成绕过校验的来源。
- `MEM-025`：内容扫描不能作为唯一安全边界；记忆不能改变工具权限、平台规则或数据归属。内部标签、
  平台说明和原始上下文块不得泄漏到完整或流式输出，分片边界也不例外。任何存储或服务接入都不能
  绕过容量、版本冲突、条件撤销、删除和隐私约束。

## API 与工具

- `MEM-030`：认证 API 支持按 target 列表、add/replace/remove/batch、容量统计，以及按 change id
  undo。返回字段仅包含自然语言条目、版本、时间、容量和必要的 change 元数据。
- `MEM-031`：Agent 工具与 API 使用同一业务服务；用户 run 与 memory-review 可写，Watch run 只读。
- `MEM-032`：Memory 存储不可用时，Memory 工具和 API 明确失败；普通对话可继续，但不得声称已经
  记住、修改、删除或撤销。
- `MEM-033`：MEMORY/USER 是个人上下文而不是社区、网络或历史来源，不能产生 source handle，不能
  伪装成结构化来源卡。

## 验收标准

- `MEM-A01`：覆盖两个 target 的 add/replace/remove/batch、容量边界、规范化去重和版本冲突。
- `MEM-A02`：覆盖敏感信息与提示注入扫描、用户隔离、删除后不再注入以及 undo CAS。
- `MEM-A03`：覆盖普通写不热更新 prompt、冷对话拼接/compact 加载最新值、未压缩恢复字节稳定。
- `MEM-A04`：覆盖十回合触发、16 轮/600k 限制、前台抢占、主会话隔离和 `memory_changed` 不计未读。
- `MEM-A05`：覆盖存储故障不谎报成功，以及 Memory 永远不能作为 source card。
- `MEM-A06`：覆盖 Memory 不进入 system、sidecar 字节稳定、标签注入转义、跨 chunk scrub，以及旧格式
  prompt snapshot 继续按旧字节恢复直到下一 prompt epoch。
