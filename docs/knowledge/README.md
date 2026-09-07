---
title: esx knowledge governance
owner: human
status: approved
agent_write_policy: human-authorized
authorization_mode: conversation
protected_paths:
  - AGENTS.md
  - docs/INDEX.md
  - docs/knowledge/README.md
  - docs/knowledge/templates/
  - docs/knowledge/intent/
  - docs/knowledge/spec/
legacy_upstream:
  - AGENTS.md
---

# 五层知识总路由

正式链路为 `INT -> SPEC -> DES -> IMP <-> EVD`。意图定义价值与边界，规格定义可验收要求，设计说明
组件协作与取舍，实现矩阵记录逐条状态，证据记录特定提交上的实际验证。源码、契约、配置与测试是事实权威。

## 权限

意图与规格的语义由人类决定；当前对话的明确要求可授权 agent 执笔，只有人类批准才能标记 `approved`。
未获批准的建议进入 proposals，不能充当正式上游。`role: baseline` 仅说明来源，不是生命周期或符合性。
不得从当前代码反向改写要求，不得把迁移、历史 passed 或结构检查当成业务完成。

## 按领域读取

| 领域 | 规格 | 设计 | 实现 |
| --- | --- | --- | --- |
| 社区 | [核心](spec/SPEC-community-core.md) | [后端](design/DES-content-community-backend.md) | [社区](implementation/IMP-community-core.md) |
| 发现 | [发现](spec/SPEC-content-discovery.md) | [后端](design/DES-content-community-backend.md) | [发现](implementation/IMP-content-discovery.md) |
| Agent | [Agent](spec/SPEC-assistant-agent.md) | [运行时](design/DES-assistant-agent-runtime.md)、[研究](design/DES-agent-community-research.md) | [Agent](implementation/IMP-assistant-agent.md) |
| Memory | [Memory](spec/SPEC-agent-memory.md) | [运行时](design/DES-assistant-agent-runtime.md)、[治理](design/DES-agent-capability-governance.md) | [Memory](implementation/IMP-agent-memory.md) |
| Watch | [Watch](spec/SPEC-agent-watch.md) | [运行时](design/DES-assistant-agent-runtime.md) | [Watch](implementation/IMP-agent-watch.md) |
| 可靠性 | [可靠性](spec/SPEC-feedback-reliability.md) | [后端](design/DES-content-community-backend.md) | [可靠性](implementation/IMP-feedback-reliability.md) |

[意图](intent/README.md)、[证据](evidence/README.md)、[操作指南](guides/README.md)、[开放门禁](status/open-gates.md)。

先定位相关条款，再读该领域设计、实现矩阵和相关覆盖组，不默认遍历所有页面。

## 单一事实

- 正式页直接位于对应层目录，文件名等于稳定 ID，保留 `id/layer/title/status/owner/updated_at`。
- INT 的上游为空；SPEC 保存 INT 上游。条款在 SPEC 的可见列表或表格中定义，ID 不随移动或措辞调整重建。
- 当前 DES 只维护 `tracks`；其 SPEC 上游按条款归属推导。同一条款可由多个设计承接。
- 当前 IMP 只维护 `code_paths` 和一个权威矩阵；每条要求唯一归属一个 IMP，矩阵的 design 是主承接设计。
  `tracks`、上游设计、证据反向列表和页面符合性摘要均由工具推导，不再重复填写。
- IMP 页头 `status` 为 `active/retired`。矩阵行使用 `aligned/unknown/diverged`；后两者写明 `gap: ...`。
  聚合按 diverged、unknown、aligned 优先级生成，不由人手维护第二份状态。
- EVD 保存观察提交、命令、scope、结果、产物及 `coverage`。每个覆盖组只有 `requirements`、`paths` 两项；
  证据上游、条款并集及 IMP/EVD 双向关系自动推导。覆盖组通常按领域拆分，同次验证的公共信息只写一次。

| 层 | 生命周期 | 其他结论 |
| --- | --- | --- |
| INT / SPEC | draft / approved / retired | human 语义所有权 |
| DES | draft / active / blocked / superseded | 可选 baseline 来源 |
| IMP | active / retired | 符合性只看逐条矩阵 |
| EVD | active / superseded | result: passed / partial / failed / blocked |

## 当前证明与历史结果

EVD 的结果属于观察提交，不因后续代码变化被改写。`active` 不等于自动证明当前 HEAD。
`aligned` 行必须找到 active/passed EVD 中覆盖该条款且仍有效的组。仅受影响的组要求重验，其他组继续有效。
未用于当前 aligned 声明的历史证据过期不阻断日常检查；unknown/diverged 的 gap 仍必须明确。

覆盖组保存观察时的输入路径快照，不能用今天更窄的路径代替。检查包括路径内容的提交、暂存、工作树和
未跟踪变化、相关条款定义变化，以及新增实现输入；源码、测试、配置与共享依赖应列入消费它们的组。
变更不可比较、提交不可达、条款在观察提交不存在或范围被缩小，都不能取得当前证明。
旧 partial 记录没有输入快照时，空 paths 明确表示未知；passed 覆盖组不允许空路径。

scope 仅取 `static/unit/integration/e2e/browser/device/synthetic/human-review/live-provider/production`。
各范围不能互相替代；只有临时 `/tmp` 产物不能支撑 passed 结论。命令必须实际运行，不因声明而自动执行。
先在最终提交运行验证，再以证据提交记录结果。缺少新证据时登记 gap，禁止自动升级符合性。

## 解析、索引与跨仓

YAML 使用安全加载，拒绝重复键；Markdown 使用 CommonMark AST 并启用表格。代码、注释与普通引用不会
建立要求，不再维护自制 Markdown 语法。当前 IMP 统一使用 `requirement | design | state | evidence or gap`。
层级 README 中的生成区块由 `knowledge-index` 更新；区块外可维护说明。检查只读，生成漂移须显式修正。
旧目录与退役 ID 留作历史，不重复迁移或改写旧提交。

跨仓语义依赖仍写 `external_upstream: repo@<40sha>:<formal-ID-or-requirement-ID>` 的非空 YAML 列表。
根仓读取子仓导出的 JSON 清单，核对 gitlink、目标唯一性与提交可达性，不解析子仓 Markdown。
历史导出使用当前工具读取 Git blob，不执行历史脚本，也不把缺失历史资料补造成当前证明。

## 命令

首次运行 `make knowledge-setup`；工具依赖安装在被忽略的 `.venv-knowledge`，可用 `KNOWLEDGE_PYTHON` 覆盖。
`make knowledge-index` 更新机械索引；`make knowledge-export REF=<sha>` 只读输出 `schema_version: 1` JSON。
公共门禁为 `make engineering-lint`，含知识与仓库约束测试。模板见 [templates](templates/)，历史材料不参与当前覆盖证明。
