package fanout

import (
	"context"

	sharedfanout "esx/app/feed/internal/fanout"
	"esx/app/feed/rpc/internal/model"
	"esx/app/feed/rpc/internal/svc"
)

// PostPublished 是手动触发 fanout 的帖子。
type PostPublished = sharedfanout.PostPublished

// HandlePostPublished 供 FanoutPost RPC 使用，复用 feed-consumer 的共享 fanout 流程：
// 写 outbox，非大 V 作者再推送到粉丝 inbox；作者资料缺失时写完 outbox 后返回错误。
func HandlePostPublished(ctx context.Context, svcCtx *svc.ServiceContext, event PostPublished) (int64, error) {
	return sharedfanout.HandlePostPublished(ctx, modelStore{svcCtx: svcCtx},
		svcCtx.UserService, svcCtx.BigVThreshold, svcCtx.FanoutBatchSize, event)
}

// modelStore 把共享流程的写入映射到 RPC 服务的 outbox / inbox 模型。
type modelStore struct {
	svcCtx *svc.ServiceContext
}

// InsertOutbox 写入作者 outbox 行。
func (s modelStore) InsertOutbox(ctx context.Context, event PostPublished) error {
	return s.svcCtx.OutboxModel.InsertIgnore(ctx, &model.FeedOutbox{
		AuthorId: event.AuthorId, PostId: event.PostId, CreatedAt: event.CreatedAt,
	})
}

// InsertInbox 为每个粉丝生成一行 inbox 并批量写入。
func (s modelStore) InsertInbox(ctx context.Context, event PostPublished, followerIDs []int64) (int64, error) {
	rows := make([]*model.FeedInbox, 0, len(followerIDs))
	for _, id := range followerIDs {
		rows = append(rows, &model.FeedInbox{
			UserId: id, AuthorId: event.AuthorId, PostId: event.PostId, CreatedAt: event.CreatedAt,
		})
	}
	return s.svcCtx.InboxModel.BatchInsertIgnore(ctx, rows)
}
