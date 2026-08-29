# 规范层

本目录收录由人类开发者决定语义的项目规范。

- `owner: human` 表示语义决定权属于人类；agent 默认可以编辑和维护。
- 修改前只需获得当前对话中的人类自然语言授权，不需要授权文件、签名或额外审批记录。
- 授权只覆盖明确指示的内容和必要索引；需要补充新语义时必须再次询问。
- 规范描述可观察行为、约束、兼容边界和验收标准，不指定内部实现方案。
- 每份 `SPEC-*` 必须引用对应 `INT-*`；只有 `status: approved` 的规范可约束活跃设计。
- 只有人类明确接受、批准或要求发布正式规范时，agent 才能将状态设为 `approved`。
- 未获授权的 agent 建议必须写入 `../proposals/`。

创建页面时使用 `../templates/spec.md`，并在本页维护索引。

## 已批准规范

| 规范 | 覆盖范围 | 状态 |
| --- | --- | --- |
| [SPEC-community-core](SPEC-community-core.md) | 用户、内容、互动、关系和交流 | approved |
| [SPEC-content-discovery](SPEC-content-discovery.md) | 关注流、搜索和个性化推荐 | approved |
| [SPEC-assistant-agent](SPEC-assistant-agent.md) | 消息虚拟线程、持久异步 Agent、工具、历史与来源 | approved |
| [SPEC-agent-memory](SPEC-agent-memory.md) | Agent 双文档自然语言记忆 | approved |
| [SPEC-agent-watch](SPEC-agent-watch.md) | Watch 主动 Assistant 私信 | approved |
| [SPEC-feedback-reliability](SPEC-feedback-reliability.md) | 行为数据闭环、可观测性和故障降级 | approved |
| [SPEC-grounded-assistant](SPEC-grounded-assistant.md) | 旧同步证据化回答 | retired / deprecated |
| [SPEC-assistant-agent-mode](SPEC-assistant-agent-mode.md) | 旧同步模式化 Agent | retired / deprecated |

当前五份 approved 规范均引用 `INT-content-community-backend`，共同构成设计层正式上游；两份
retired 规范只保留历史契约，不能再约束活跃设计。

2026-08-29 修订：Assistant 迁入消息页虚拟线程，统一为通用持久异步 Agent；新增 MySQL lease
恢复、Hermes 双文档记忆、compact/BM25、Watch 主动消息和可选来源 ledger，硬删除旧 chat/mode
与 Watch hits 契约。`SPEC-assistant-agent` 接替两份 retired Assistant 规范。

2026-08-27 修订：意图将 Agent 扩张为内容域搜索综述、结构化记忆、可解释推荐与条件追踪。
`SPEC-grounded-assistant` 批准评论作为社区证据；`SPEC-assistant-agent-mode` 改为分组
白名单与版本化授权；新增 `SPEC-agent-memory`、`SPEC-agent-watch`。不引入游戏库、价格、
商城或通用通知中心。

2026-08-26 新增：`SPEC-assistant-agent-mode` 获人类批准；意图边界同步放宽为受授权、
工具白名单与执行预算约束的受限形态；原检索问答管线命名 enhanced_search（ASST-043）。

2026-08-15 锁定：补齐意图模板字段；记录 `/api/v1` 帖子写接口移除；明确详情互动状态、
不可用目标点赞、曝光客户端/服务端分工、`Total` 估计语义、人类评测门禁和 `REL-033`/
`REL-054` 编号。站内赞/评/关通知不在范围。
