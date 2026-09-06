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

```text
INT（human） -> SPEC（human） -> DES（agent） -> IMP（agent） <-> EVD（agent）
```

INT 定义产品价值与边界；SPEC 定义可验收约束、质量指标和失败行为；DES 说明实现方案与取舍；
IMP 把每个生效 requirement 唯一映射到代码和当前状态；EVD 记录某个提交上实际执行的验证。
源码、配置、`.api`、`.proto`、迁移与测试结果始终是当前行为的事实权威。

## 权限与决策

`protected_paths` 表示人类语义所有权，不是永久只读。当前对话中的自然语言指令就是有效授权，无需
签名或仓库内授权文件；授权只覆盖明确目标及必要索引。范围不清、上层冲突或需要新增产品语义时，
agent 必须停止并请求决定。未获授权的 INT/SPEC 建议只进入 [proposals](proposals/README.md)，提案不能
作为正式上游。只有人类明确批准后，INT/SPEC 才能标为 `approved`；agent 执笔不改变 `owner: human`。

## 结构契约

五层正式页面均使用稳定 kebab-case ID、目录索引和以下 frontmatter：

- 页面必须是对应层目录的直接子项，文件名严格为 `<id>.md`，并在该层 README 中恰好登记一次；
- 通用字段：`id`、`layer`、`title`、`status`、`owner`、`upstream`、`updated_at`；
- `title` 和所有非空列表项不得为空白，关键列表不得重复，`updated_at` 必须是真实日历日期；
- 可选 `role: baseline` 只说明迁移角色，不是生命周期或合规状态；
- DES 与 IMP 的 `tracks` 只能列 approved SPEC 中逐条出现的精确 requirement ID；
- IMP 另需 `code_paths` 和 `evidence`，不得保存提交或验证日期；
- EVD 另需 `covers`、`scope`、`commands`、`observed_commit` 和 `result`，可选 `artifacts`。

正式 requirement 只从 Markdown 可见正文中的规范 bullet/table 提取；frontmatter、合法 fence、HTML
comment 与 inline code span 内容不参与。跨行 span 只由与 opener 等长的 maximal backtick run 闭合，
lookahead 遇空行、ATX heading、Setext heading underline 或合法 fence opener 即停止。行首三个以上
backtick、但 remainder 又含 backtick 的非法 fence-shaped 行只允许同行 span，不得延伸到后续物理行。

合法状态：

| 层 | status / result |
| --- | --- |
| INT / SPEC | `draft`、`approved`、`retired` |
| DES | `draft`、`active`、`blocked`、`superseded` |
| IMP | `unknown`、`aligned`、`diverged`、`retired` |
| EVD status | `active`、`superseded` |
| EVD result | `passed`、`partial`、`failed`、`blocked` |

引用方向固定：SPEC→INT、DES→SPEC、IMP→DES、EVD→IMP。跨仓正式依赖使用
`external_upstream: repo@<40sha>:<formal-ID-or-requirement-ID>`；键存在时不得为空。过渡 DES 可引用
白名单中的 `legacy:<path>#<heading>`，但不能据此创造新产品语义。

## 对齐与证据

- 每个 approved requirement 至少由一个 current DES 跟踪，并且恰有一个非 retired IMP owner；台账
  每行只写一个 requirement、一个 DES、一个状态和一个 `EVD-*` 或 `gap: ...`。
- IMP 页头必须等于台账聚合：任一行 `diverged` 则页头 diverged；否则任一行 `unknown` 则 unknown；
  全部行 aligned 才可 aligned。`diverged/unknown` 行必须说明 gap。
- aligned 行必须由该 IMP 双向登记的 active/passed EVD 覆盖。EVD 的 40 位 `observed_commit` 必须可达；
  任一 upstream IMP 的 `code_paths` 相对该提交发生已提交、暂存、工作树或未跟踪变更时，旧 passed EVD
  自动失效。命令必须真实执行；scope 限于 `static/unit/integration/e2e/browser/device/synthetic/`
  `human-review/live-provider/production`。
- 合成数据只能证明 synthetic 范围，旧 [implementation/evidence](implementation/evidence/README.md)
  只作 legacy 历史，均不能关闭当前人类评审、真实 provider、设备或生产门禁。

模板见 [templates](templates/)，迁移登记见 [TRANSITION](TRANSITION.md)，操作指南与现场状态分别见
[guides](guides/README.md) 和 [status](status/README.md)。公共检查入口为 `make engineering-lint`。
