package logic

import (
	"context"

	"esx/app/feed/mq/internal/model"
	"user/userservice"

	"google.golang.org/grpc"
)

type PostPublished struct {
	PostId    int64
	AuthorId  int64
	CreatedAt int64
}

type OutboxInserter interface {
	InsertIgnore(ctx context.Context, row *model.FeedOutbox) error
}

type InboxBatchInserter interface {
	BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error)
}

type UserGetter interface {
	GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error)
	GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error)
}

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
		return 0, err
	}
	if err := outbox.InsertIgnore(ctx, &model.FeedOutbox{
		AuthorId: event.AuthorId, PostId: event.PostId, CreatedAt: event.CreatedAt,
	}); err != nil {
		return 0, err
	}
	if userResp.User == nil || userResp.User.FollowerCount >= bigVThreshold {
		return 0, nil
	}
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
			return 0, err
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
