# 意图层

本目录收录由人类开发者决定语义的项目意图。

- `owner: human` 表示语义决定权属于人类；agent 默认可以编辑和维护。
- 修改前只需获得当前对话中的人类自然语言授权，不需要授权文件、签名或额外审批记录。
- 授权只覆盖明确指示的内容和必要索引；需要补充新语义时必须再次询问。
- 意图说明产品目标、成功标准、边界和非目标，不描述具体技术方案。
- 只有 `status: approved` 的 `INT-*` 页面才能约束规范和后续工作。
- 只有人类明确接受、批准或要求发布正式意图时，agent 才能将状态设为 `approved`。
- 未获授权的 agent 建议必须写入 `../proposals/`。

创建页面时使用 `../templates/intent.md`，并在本页维护索引。

## 当前意图

- [INT-content-community-backend](INT-content-community-backend.md)：小白盒内容社区后端（approved）
