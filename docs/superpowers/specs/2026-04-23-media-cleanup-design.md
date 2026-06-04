# 媒体资源治理修复设计

**日期**: 2026-04-23
**范围**: app/media
**关联审查项**: H7 (孤儿 S3 对象、DB 自增 ID)

---

## 问题描述

### 孤儿 S3 对象

`DeleteMedia` 软删媒体记录（`status = 0`）后，S3 存储桶中的实际文件对象未被清理。长期运行后产生大量无引用的孤儿对象，占用存储空间。

### DB 自增 ID

Media 服务使用数据库自增 ID 作为媒体记录主键。在分布式场景下：
- 自增 ID 存在单点瓶颈（写入必须串行）
- 不同分片的 ID 可能冲突（若未来分库分表）
- ID 可预测，不利于安全（如遍历他人媒体）

项目其他模块（Content 的 Post、User 的 Profile）已使用 `util.NextID()` 雪花 ID。

## 目标

1. 媒体软删后异步清理 S3 对象
2. 新创建媒体使用雪花 ID 替代自增 ID

## 方案

### 1. 异步清理孤儿 S3 对象

**不做实时同步删除的原因**：
- S3 删除是网络 IO，实时删除会增加 RPC 延迟
- 若删除失败不应阻塞业务响应
- 批量清理效率更高

**方案**：事件驱动异步清理

在 `DeleteMediaLogic` 中，软删成功后发送 RocketMQ 消息：

```go
// DeleteMedia 成功后
l.svcCtx.MQProducer.Send(&mqx.MediaDeletedEvent{
    MediaId:  in.MediaId,
    S3ObjectKey: m.ObjectKey,
    Bucket:   l.svcCtx.Config.S3Storage.Bucket,
})
```

消费者 `media_cleanup_consumer.go`：
- 监听 `media_deleted` topic
- 调用 S3 客户端 `RemoveObject` 删除对象
- 若失败进入死信队列，人工介入或定时重试

由于当前项目已有 `mqx` 包和 RocketMQ 依赖，复用现有基础设施。

### 2. 雪花 ID 替换自增 ID

**范围**：仅影响新创建的记录。存量数据保留自增 ID，避免迁移成本。

在 `CreateMediaLogic` 中：

```go
mediaId, err := util.NextID()
if err != nil {
    return nil, errx.NewWithCode(errx.SystemError)
}

_, err = l.svcCtx.MediaModel.Insert(l.ctx, &model.Media{
    Id:     mediaId, // 使用雪花 ID
    UserId: in.UserId,
    // ...
})
```

需要确认 `Media` 表的 `id` 字段类型为 `BIGINT`（雪花 ID 为 int64），且未设置 `AUTO_INCREMENT`。若当前表结构为 `AUTO_INCREMENT`，需要修改表结构移除自增属性。

**Migration**：
```sql
ALTER TABLE media MODIFY id BIGINT NOT NULL;
```

（注：若已有自增数据，新插入的雪花 ID 需确保大于当前最大自增 ID，避免冲突。雪花 ID 基于时间戳，通常远大于已有自增 ID。）

## 文件变更

| 文件 | 变更 |
|------|------|
| `app/media/internal/logic/delete_media_logic.go` | 软删成功后发送 MQ 事件 |
| `app/media/internal/mqs/media_cleanup_consumer.go` | 新增消费者，执行 S3 删除 |
| `app/media/internal/logic/create_media_logic.go` | 使用 `util.NextID()` |
| `app/media/internal/config/config.go` | 确认 MQ producer 在 ServiceContext 中 |
| `deploy/sql/migration/xxx_remove_media_auto_increment.sql` | 移除自增属性（若需要） |

## 验收标准

- 删除媒体后，MQ 消息成功投递
- 消费者正确处理消息并删除 S3 对象
- 新创建媒体的 ID >= 1<<22（雪花 ID 最小值特征）
- 所有测试通过

## 依赖

- 需要 `mqx` 包支持事件投递（项目中已有）
- 若 MQ 基础设施未完全就绪，可降级为在 `DeleteMedia` 中启动 goroutine 异步删除（需确保 goroutine 内使用 context 拷贝）
