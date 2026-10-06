package logic

import (
	"context"
	"fmt"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/feed/mq/internal/model"
	"esx/app/user/rpc/userservice"

	"esx/pkg/logging"
)

// PostPublished 是触发 fanout 的已发布帖子。
type PostPublished struct {
	PostId    int64
	AuthorId  int64
	CreatedAt int64
}

// OutboxInserter 写作者 outbox。
type OutboxInserter interface {
	InsertIgnore(ctx context.Context, row *model.FeedOutbox) error
}

// InboxBatchInserter 批量写粉丝 inbox。
type InboxBatchInserter interface {
	BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error)
}

// UserGetter 读取作者资料与粉丝列表。
type UserGetter interface {
	GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...callopt.Option) (*userservice.GetUserResp, error)
	GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...callopt.Option) (*userservice.GetFollowersResp, error)
}

// HandlePostPublished 先写 outbox，再对非大 V 作者把帖子推送到全部粉丝的 inbox，返回新增的 inbox 行数。
// 大 V 只写 outbox，读关注流时再拉取，避免一次发帖产生海量写入。
func HandlePostPublished(
	ctx context.Context,
	outbox OutboxInserter,
	inbox InboxBatchInserter,
	userSvc UserGetter,
	bigVThreshold int64,
	fanoutBatchSize int64,
	event PostPublished,
) (int64, error) {
	userResp, err := userSvc.GetUser(ctx, &userservice.GetUserReq{UserId: event.AuthorId})
	if err != nil {
		return 0, fmt.Errorf("fanout: get user %d: %w", event.AuthorId, err)
	}
	// outbox 先于作者资料校验写入：拉模式读取只依赖 outbox，不受推送是否成功影响。
	if err := outbox.InsertIgnore(ctx, &model.FeedOutbox{
		AuthorId: event.AuthorId, PostId: event.PostId, CreatedAt: event.CreatedAt,
	}); err != nil {
		return 0, fmt.Errorf("fanout: insert outbox for post %d: %w", event.PostId, err)
	}
	if userResp.User == nil {
		logging.WithContext(ctx).Errorw("fanout: GetUser returned nil user",
			logging.Field("author_id", event.AuthorId), logging.Field("post_id", event.PostId))
		return 0, fmt.Errorf("fanout: nil user for author %d", event.AuthorId)
	}
	if userResp.User.FollowerCount >= bigVThreshold {
		return 0, nil
	}
	// 分页拉取全部粉丝后一次性写入 inbox。
	pageSize := int32(fanoutBatchSize)
	if pageSize <= 0 {
		pageSize = 500
	}
	rows := make([]*model.FeedInbox, 0)
	var fetched int64
	for page := int32(1); ; page++ {
		followersResp, err := userSvc.GetFollowers(ctx, &userservice.GetFollowersReq{
			UserId: event.AuthorId, Page: page, PageSize: pageSize,
		})
		if err != nil {
			return 0, fmt.Errorf("fanout: get followers page %d for author %d: %w", page, event.AuthorId, err)
		}
		for _, user := range followersResp.Users {
			if user.Id > 0 {
				rows = append(rows, &model.FeedInbox{
					UserId: user.Id, AuthorId: event.AuthorId,
					PostId: event.PostId, CreatedAt: event.CreatedAt,
				})
			}
		}
		fetched += int64(len(followersResp.Users))
		if len(followersResp.Users) == 0 || int32(len(followersResp.Users)) < pageSize || fetched >= followersResp.Total {
			break
		}
	}
	return inbox.BatchInsertIgnore(ctx, rows)
}
