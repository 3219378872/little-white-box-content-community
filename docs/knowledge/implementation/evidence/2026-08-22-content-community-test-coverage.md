---
implementation: IMP-content-community-backend
verified_at: 2026-08-22
verified_commit: bd4457c
commands:
  - go test ./app/media/rpc/internal/logic/ -count=1
  - go vet -tags=integration ./app/media/rpc/internal/logic/
  - make test
  - make coverage
  - go test -tags=integration -count=3 ./app/content/rpc/internal/model/
  - go test -tags=integration -count=2 ./app/interaction/rpc/internal/model/
result: passed
---

# 2026-08-22 测试覆盖补强（P1 media 上传 / P2 薄弱 logic 失败路径 / P3 content+interaction model 集成）

## 背景

覆盖率审查发现基线门禁全过但结构性缺口：media 上传 logic 整文件零覆盖、
9 个薄弱 logic/mq 文件失败路径缺失、content/interaction 数据访问层既无单测也无集成测试。

## 改动

- `baecb48` test(media)：UploadImage/UploadVideo 单测。内存版 `ObjectStorage`
  与脚本化 gRPC client-streaming fake；覆盖成功路径落库字段断言、幂等重试
  （孤儿对象清理 + best-effort 删除失败容忍）、幂等冲突、参数/类型/压缩/
  上传失败、nil command model、非法 TempSink limit。
- `9e48d61` test(logic)：10 个文件补失败路径。
  user：register 手机号全路径、refresh session JTI 白名单轮换故障注入、
  verify-code 三维频控 Redis 故障注入（`flakyRedis` 按 key 注入）；
  content-mq count-sync 消费者无效事件/批量中断/构造期配置校验；
  gateway 帖子详情 RPC 与 viewer-state 失败、作者信息降级边界；
  recommend similar-posts 参数校验/召回不可用/部分降级无候选/可见性检查
  失败/推理降级标记；gateway assistant 流式事件错误映射与终态语义；
  embedding consumer revision 读取与 delete 类型重试路径；
  content delete-post 事务冲突与缓存失效容忍。
- `bd4457c` test(model)：content + interaction model 集成测试，复用
  user/recommend 的 testcontainers 模式（`testutil.SetupTestEnvM` +
  `//go:build integration`）。覆盖 post/comment/tag/post_tag/category
  CRUD、revision 守卫的 post 命令（CreatePost 幂等绑定 + outbox、
  UpdatePost 字段白名单、DeletePost 条件软删）、评论计数同事务调整；
  interaction Like/Favorite 命令流（action_count 一致性、ErrNoStateChange
  幂等）、like/favorite 记录条件更新、action_count 递减下限保护。

## 结果

- `make test`（race）全绿；`make coverage` 基线门禁 exit 0：
  handler 88.9≥88、logic **84.4**≥76（原 78.6）、model 12.7≥10、
  mq_consumer **78.5**≥72（原 73.9）、shared 57.4≥42。
- 目标门禁仍预期未达（logic 距目标 85 差 0.6pt）；model 层单测口径不变，
  新增数据访问保护在 `-tags=integration` 口径下生效（按用户决策暂不进 CI）。
- 关键文件覆盖率：upload_image_logic 86.8%、upload_video_logic 85.7%、
  register_logic 90%（registerByPhone）、refresh_session rotate/store 100%、
  send_verify_code 95.8%、get_post_logic 100%/93.3%、count_sync_consumer
  consume 100%、get_similar_posts 91.1%、delete_post_logic 93.1%、
  assistant_chat/embedding_consumer 主函数 100%。
- 集成测试 `-tags=integration` 定向跑通过（content model 连续 3 次、
  interaction 连续 2 次），并验证与既有 integration tag 文件编译共存。

## 未覆盖边界

- 集成测试依赖 Docker/testcontainers，CI 仅跑 `integration-critical`
  （behavior pipeline + interaction/user logic+model），本次新增两个
  model 包未纳入 critical 集——回归保护目前依赖本地执行。
- 少数防御性分支不可确定性触发（GenerateToken 失败、crypto/rand 失败、
  ctx 取消穿透 recall/enrich/inference 等），保留未覆盖。
- 外部门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
