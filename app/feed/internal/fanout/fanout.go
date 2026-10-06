// Package fanout 是 feed-consumer 与 FanoutPost RPC 共用的帖子推送流程；
// 两端各自用本服务的模型实现 Store，流程与错误语义只维护这一份。
package fanout

import (
	"context"
	"fmt"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/user/rpc/userservice"

	"esx/pkg/logging"
)

// defaultBatchSize 是未配置 FanoutBatchSize 时拉取粉丝的分页大小。
const defaultBatchSize = 500

// PostPublished 是触发 fanout 的已发布帖子。
type PostPublished struct {
	PostId    int64
	AuthorId  int64
	CreatedAt int64
}

// Store 写作者 outbox 与粉丝 inbox；InsertInbox 返回新增的 inbox 行数。
type Store interface {
	InsertOutbox(ctx context.Context, event PostPublished) error
	InsertInbox(ctx context.Context, event PostPublished, followerIDs []int64) (int64, error)
}

// UserGetter 读取作者资料与粉丝列表。
type UserGetter interface {
	GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...callopt.Option) (*userservice.GetUserResp, error)
	GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...callopt.Option) (*userservice.GetFollowersResp, error)
}

// HandlePostPublished 先写 outbox，再对非大 V 作者把帖子推送到全部粉丝的 inbox，返回新增的 inbox 行数。
// 大 V 只写 outbox，读关注流时再拉取，避免一次发帖产生海量写入；作者资料缺失时返回错误。
func HandlePostPublished(
	ctx context.Context,
	store Store,
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
	if err := store.InsertOutbox(ctx, event); err != nil {
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
	followerIDs, err := listFollowerIDs(ctx, userSvc, event.AuthorId, fanoutBatchSize)
	if err != nil {
		return 0, err
	}
	return store.InsertInbox(ctx, event, followerIDs)
}

// listFollowerIDs 分页拉取作者的全部粉丝 ID，跳过无效 ID。
func listFollowerIDs(ctx context.Context, userSvc UserGetter, authorID, batchSize int64) ([]int64, error) {
	pageSize := int32(batchSize)
	if pageSize <= 0 {
		pageSize = defaultBatchSize
	}
	ids := make([]int64, 0)
	var fetched int64
	for page := int32(1); ; page++ {
		followersResp, err := userSvc.GetFollowers(ctx, &userservice.GetFollowersReq{
			UserId: authorID, Page: page, PageSize: pageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("fanout: get followers page %d for author %d: %w", page, authorID, err)
		}
		for _, user := range followersResp.Users {
			if user.Id > 0 {
				ids = append(ids, user.Id)
			}
		}
		fetched += int64(len(followersResp.Users))
		if len(followersResp.Users) == 0 || int32(len(followersResp.Users)) < pageSize || fetched >= followersResp.Total {
			return ids, nil
		}
	}
}
