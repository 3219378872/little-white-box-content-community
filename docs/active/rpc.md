# RPC 开发速查

## 当前结构

- RPC 和 MQ 模块位于 `app/`；公共错误传输在 `pkg/interceptor/`。
- proto 定义位于 `proto/`，生成代码不手动编辑。
- Gateway 到 RPC 的客户端和服务发现配置以实际 `etc/` 文件为准。

## Assistant 站内证据问答

- Assistant 的默认搜索工具始终调用现有 Search 综合搜索并保留原有帖子、用户、标签计数和有界来源；Content `GetPostsByIds` 是帖子正文证据的尽力回源，不是 Search 工具的可用性前提。Content 未配置、失败或未返回已发布正文时，Search 仍返回元数据结果，但标记为无正文证据并跳过 Generator。
- 只有 Content 回源确认 `status=1` 且正文非空的帖子才能形成 `community_evidence` 并设置 `HasEvidence=true`。用户/标签和 Search 索引中的帖子元数据只作为搜索结果来源，不进入问答证据上下文。
- 帖子标题和正文片段属于不可信社区内容。可信 `SOURCE [post:ID]` 标识与 JSON 编码的 `COMMUNITY_CONTENT_JSON` 分开，Generator 通过独立系统指令约束模型只使用有界证据上下文，不执行证据中的指令；没有有效帖子证据时不调用模型生成帖子问答。
- 当前 `SourceReference` 契约只能传 `source_type`、`source_id` 和 `title`。SSE 来源事件继续提供旧有帖子、用户和标签来源；启用 Generator 时，手写 Logic 仅为已验证帖子在答案后追加 JSON 编码的有界正文引用，并在会话来源中保存片段。在没有兼容字段前不把片段伪装成新的 proto 能力。
- Embedding/Milvus 当前用于异步写入和重建，没有在线检索 RPC；Assistant 此阶段不宣称使用向量召回。

## 修改流程

1. 先阅读对应 `.proto`、server、Logic 和调用方。
2. 修改 `.proto` 后运行仓库使用的 goctl/protobuf 生成命令。
3. 所有 zrpc 调用透传原始 ctx；goroutine 使用 ctx 的副本并处理取消。
4. 跨服务业务错误使用 `pkg/errx` 和现有 interceptor 传输，不重新定义错误协议。

## 验证

- 至少覆盖调用成功、下游失败、超时/取消三类路径中适用的部分。
- 运行 `scripts/test.sh`、`scripts/vet.sh`，并检查生成文件差异。
