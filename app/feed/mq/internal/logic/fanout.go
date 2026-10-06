package logic

import (
	"context"

	"esx/app/feed/internal/fanout"
	"esx/app/feed/mq/internal/model"
)

// PostPublished 是触发 fanout 的已发布帖子。
type PostPublished = fanout.PostPublished

// UserGetter 读取作者资料与粉丝列表。
type UserGetter = fanout.UserGetter

// OutboxInserter 写作者 outbox。
type OutboxInserter interface {
	InsertIgnore(ctx context.Context, row *model.FeedOutbox) error
}

// InboxBatchInserter 批量写粉丝 inbox。
type InboxBatchInserter interface {
	BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error)
}

// HandlePostPublished 用 consumer 的模型执行共享 fanout 流程，返回新增的 inbox 行数。
func HandlePostPublished(
	ctx context.Context,
	outbox OutboxInserter,
	inbox InboxBatchInserter,
	userSvc UserGetter,
	bigVThreshold int64,
	fanoutBatchSize int64,
	event PostPublished,
) (int64, error) {
	return fanout.HandlePostPublished(ctx, modelStore{outbox: outbox, inbox: inbox},
		userSvc, bigVThreshold, fanoutBatchSize, event)
}

// modelStore 把共享流程的写入映射到 consumer 的 outbox / inbox 模型。
type modelStore struct {
	outbox OutboxInserter
	inbox  InboxBatchInserter
}

// InsertOutbox 写入作者 outbox 行。
func (s modelStore) InsertOutbox(ctx context.Context, event PostPublished) error {
	return s.outbox.InsertIgnore(ctx, &model.FeedOutbox{
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
	return s.inbox.BatchInsertIgnore(ctx, rows)
}
