# 媒体资源治理修复计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans.

**Goal:** 媒体软删后异步清理 S3 对象，新创建媒体使用雪花 ID 替代自增 ID。

**Architecture:** DeleteMedia 发送 RocketMQ 事件，Consumer 异步删除 S3 对象；CreateMedia 调用 util.NextID() 生成 ID。

**Tech Stack:** Go 1.26.1, go-zero, RocketMQ, MinIO/S3

---

## 文件变更总览

| 文件 | 变更 |
|------|------|
| `app/media/internal/logic/delete_media_logic.go` | 软删成功后发送 MQ 事件 |
| `app/media/internal/logic/create_media_logic.go` | 使用 `util.NextID()` |
| `app/media/internal/mqs/media_cleanup_consumer.go` | 新增消费者 |
| `app/media/internal/svc/service_context.go` | 注入 MQ producer / consumer |
| `app/media/internal/config/config.go` | 确认 MQ 配置 |

---

### Task 1: Media 删除事件投递

**Files:**
- Modify: `app/media/internal/logic/delete_media_logic.go`
- Modify: `app/media/internal/svc/service_context.go`

- [ ] **Step 1: 确认 ServiceContext 中有 MQ Producer**

在 `app/media/internal/svc/service_context.go` 中确认：
```go
type ServiceContext struct {
	Config      config.Config
	MediaModel  model.MediaModel
	// ... other fields ...
	MQProducer  mqx.Producer // 若不存在则添加
}
```

若 `mqx.Producer` 不存在，需根据项目实际 MQ 封装添加。

- [ ] **Step 2: 在 DeleteMedia 成功后发送事件**

```go
func (l *DeleteMediaLogic) DeleteMedia(in *pb.DeleteMediaReq) (*pb.DeleteMediaResp, error) {
	// ... existing validation and conditional update ...

	result, err := l.svcCtx.MediaModel.UpdateStatus(l.ctx, in.MediaId, 1, 0)
	if err != nil {
		l.Errorw("MediaModel.UpdateStatus failed", ...)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		l.Infow("delete media no-op", logx.Field("media_id", in.MediaId))
		return &pb.DeleteMediaResp{}, nil
	}

	// 投递异步清理事件
	if l.svcCtx.MQProducer != nil {
		msg := &mqx.MediaDeletedMessage{
			MediaId:     in.MediaId,
			S3ObjectKey: m.ObjectKey,
			Bucket:      l.svcCtx.Config.S3Storage.Bucket,
			DeletedAt:   time.Now().Unix(),
		}
		if err := l.svcCtx.MQProducer.Send(ctx, "media_deleted", msg); err != nil {
			l.Errorw("send media_deleted event failed",
				logx.Field("media_id", in.MediaId),
				logx.Field("err", err.Error()),
			)
			// 不阻塞主流程，记录日志后返回成功
		}
	}

	l.Infow("delete media success", ...)
	return &pb.DeleteMediaResp{}, nil
}
```

需要添加 import：`"time"`。

- [ ] **Step 3: Commit**

```bash
git add app/media/internal/logic/delete_media_logic.go
git commit -m "feat(media): send MQ event on media soft-delete

- Asynchronously notify S3 cleanup after successful soft-delete
- Non-blocking: failure to send event does not fail the request

Refs H7"
```

---

### Task 2: S3 清理消费者

**Files:**
- Create: `app/media/internal/mqs/media_cleanup_consumer.go`

- [ ] **Step 1: 创建消费者**

```go
package mqs

import (
	"context"
	"esx/app/media/internal/svc"
	"esx/pkg/mqx"

	"github.com/zeromicro/go-zero/core/logx"
)

type MediaCleanupConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewMediaCleanupConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *MediaCleanupConsumer {
	return &MediaCleanupConsumer{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (c *MediaCleanupConsumer) Consume(ctx context.Context, key string, body []byte) error {
	var msg mqx.MediaDeletedMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		c.Errorw("unmarshal MediaDeletedMessage failed", logx.Field("err", err.Error()))
		return err // 返回错误触发重试
	}

	c.Infow("cleaning up S3 object",
		logx.Field("media_id", msg.MediaId),
		logx.Field("object_key", msg.S3ObjectKey),
	)

	if err := c.svcCtx.S3Client.RemoveObject(ctx, msg.Bucket, msg.S3ObjectKey); err != nil {
		c.Errorw("RemoveObject failed",
			logx.Field("media_id", msg.MediaId),
			logx.Field("object_key", msg.S3ObjectKey),
			logx.Field("err", err.Error()),
		)
		return err // 触发重试或死信
	}

	c.Infow("S3 object cleaned up",
		logx.Field("media_id", msg.MediaId),
	)
	return nil
}
```

- [ ] **Step 2: 在 ServiceContext 中注册消费者**

在 `app/media/media.go` 的 `main` 函数中，启动时注册消费者：

```go
func main() {
	// ...
	ctx := svc.NewServiceContext(c)

	// 注册 MQ 消费者
	if ctx.MQConsumer != nil {
		consumer := mqs.NewMediaCleanupConsumer(context.Background(), ctx)
		ctx.MQConsumer.Consume("media_deleted", consumer.Consume)
	}

	// ... 启动 RPC server ...
}
```

- [ ] **Step 3: Commit**

```bash
git add app/media/internal/mqs/media_cleanup_consumer.go
git commit -m "feat(media): add S3 cleanup consumer for deleted media

- Consume media_deleted events and remove S3 objects
- Errors trigger MQ retry / dead-letter

Refs H7"
```

---

### Task 3: 新创建媒体使用雪花 ID

**Files:**
- Modify: `app/media/internal/logic/create_media_logic.go`

- [ ] **Step 1: 修改 CreateMedia 使用 `util.NextID()`**

在 `CreateMediaLogic` 中，插入前生成雪花 ID：

```go
mediaId, err := util.NextID()
if err != nil {
	l.Errorw("NextID failed", logx.Field("err", err.Error()))
	return nil, errx.NewWithCode(errx.SystemError)
}

_, err = l.svcCtx.MediaModel.Insert(l.ctx, &model.Media{
	Id:       mediaId,
	UserId:   in.UserId,
	Type:     in.Type,
	Url:      in.Url,
	Status:   1,
	ObjectKey: in.ObjectKey,
	// ... other fields ...
})
```

- [ ] **Step 2: 确认表结构支持非自增 ID**

检查 `deploy/sql/` 或 `app/media/internal/model/` 中的表结构定义，确认 `id` 字段为 `BIGINT NOT NULL` 且没有 `AUTO_INCREMENT`。

若存在 `AUTO_INCREMENT`，添加 migration：
```sql
ALTER TABLE media MODIFY id BIGINT NOT NULL;
```

- [ ] **Step 3: Commit**

```bash
git add app/media/internal/logic/create_media_logic.go
git commit -m "feat(media): use snowflake ID for new media records

- Replace auto-increment with util.NextID()
- Aligns with Content and User modules

Fixes H7"
```

---

## 验证清单

- [ ] `go test ./app/media/... -race -cover` 通过
- [ ] 新创建媒体的 ID 为雪花 ID（长度/特征可识别）
- [ ] 删除媒体后 MQ 消息成功投递（若 MQ 可用）
