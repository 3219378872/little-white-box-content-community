---
implementation: IMP-content-community-backend
verified_at: 2026-08-30
verified_commit: 7dc602795fee18182105d67e3e0f5f32e85f38d5
commands:
  - go test ./app/assistant/... ./app/gateway/internal/handler/assistant ./app/gateway/internal/logic/assistant -count=1
  - go test -race ./app/assistant/internal/llm ./app/assistant/internal/runtime ./app/assistant/internal/tool ./app/assistant/rpc/internal/logic ./app/assistant/watch ./app/assistant/worker/internal/svc ./app/gateway/internal/handler/assistant ./app/gateway/internal/logic/assistant -count=1
  - PATH=/tmp/xbh-assistant-generate-venv/bin:$PATH make generate
  - make fmt-check
  - make vet
  - make lint
  - make check
  - git diff --check
result: passed
---

# Assistant runtime 完整性修复

## 环境

后端 worktree `task/assistant-completeness`，实现提交
`7dc602795fee18182105d67e3e0f5f32e85f38d5`。没有调用 live LLM，也没有在根真实栈重启服务。
完整生成第一次因宿主 Python 缺少仓库声明的 `grpc_tools.protoc` 在生成前退出；随后在 `/tmp`
一次性 venv 中安装固定版本 `grpcio-tools==1.71.0`，以该环境重跑 `make generate` 成功。生成 diff
仅包含 Assistant protobuf 与 Gateway Assistant types 的预期字段。

## 已验证结果

- compact token 估算排除已 compacted 消息；只保留 unmatched tool call，已完成工具轮可压缩；
  无可继续缩减的单条大消息不会立即重复 compact。
- Responses `status=incomplete` 保存 partial，并以
  `LLM_INCOMPLETE_MAX_OUTPUT_TOKENS|CONTENT_FILTER|UNKNOWN` error 终止，不写 done。
- Watch run 从 bucket 的精确 hit IDs 按 user 回源，向模型传递不可信 hit context；无当前版本 consent
  不调度。成功时 token、Assistant message、BM25 outbox、未读、bucket sent、小时/日计数与 done 同事务；
  error/cancelled bucket 重排。
- 用户附件与 `contextPostId` 进入 provider-bound `api_content` 和 queued payload，恢复/FIFO/tool session
  使用相同数据；可见正文保持原样。
- user/assistant 可见消息都生成历史索引 outbox；`present_sources` 展示前重验 post published/revision
  或 web URL，失效来源被剔除。
- memory-review 工具结果结构化持久化 `change_ids`，完成后生成 `memory_changed(changeId)` 系统行与
  event，`unread=false`，可直接调用既有 undo CAS API。
- Assistant messages 默认返回最新一页，`beforeId` 向前翻页，`afterId` 保留增量语义；两者同时传入
  返回参数错误。响应新增 `hasMore` 与 `nextBeforeId`。
- HTTP SSE 恢复游标取 query/header 最大值，事件帧输出 `id:`；25 秒 comment heartbeat 不污染持久
  event 白名单。

## 迁移验证

使用显式命名且 `--rm` 的临时 MySQL 8 容器构造已有 `assistant_runtime_v3` marker、各 1 条 message/
scheduled bucket、但缺少新列的旧 schema。两条 2026-08-30 patch 连续重放两次后：

```text
assistant_message.change_id=0
watch_delivery_bucket.not_before_ms=0
marker=1
messages=1
buckets=1
```

临时容器已停止并自动删除。该结果只证明 schema patch 幂等升级与数据保留，不等于生产迁移证据。

## 契约变更

- protobuf `ListMessagesReq.before_id = 5`。
- protobuf `ListMessagesResp.has_more = 2`、`next_before_id = 3`。
- REST query `beforeId`；REST response `hasMore`、`nextBeforeId`。
- 既有 message/event `changeId` 现在由 memory-review 权威填充；没有新增 SSE event type。

## 未覆盖边界

- 未运行 live provider、根真实浏览器 E2E 或生产迁移。
- Watch 成功事务由源码同事务边界与内存定向测试验证，尚未做 MySQL 故障注入。
- lease fencing、command journal 副作用前 reserve、delete confirmation target revision、redirect 在途取消和
  consent 撤销活跃 run 属于其它修复边界，本证据不宣称关闭。
- 历史 BM25 四种 shape、ES rebuild/delete 与 365 天回源未做真实 Elasticsearch 集成验证。
