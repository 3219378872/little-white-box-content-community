package fanout

import (
	"context"

	"esx/app/feed/internal/model"
	"esx/app/feed/internal/svc"
	"user/userservice"
)

type PostPublished struct {
	PostId    int64
	AuthorId  int64
	CreatedAt int64
}

func HandlePostPublished(ctx context.Context, svcCtx *svc.ServiceContext, event PostPublished) (int64, error) {
	userResp, err := svcCtx.UserService.GetUser(ctx, &userservice.GetUserReq{UserId: event.AuthorId})
	if err != nil {
		return 0, err
	}
	if err := svcCtx.OutboxModel.InsertIgnore(ctx, &model.FeedOutbox{AuthorId: event.AuthorId, PostId: event.PostId, CreatedAt: event.CreatedAt}); err != nil {
		return 0, err
	}
	if userResp.User == nil || userResp.User.FollowerCount >= svcCtx.BigVThreshold {
		return 0, nil
	}
	pageSize := int32(svcCtx.FanoutBatchSize)
	if pageSize <= 0 {
		pageSize = 500
	}
	followersResp, err := svcCtx.UserService.GetFollowers(ctx, &userservice.GetFollowersReq{UserId: event.AuthorId, Page: 1, PageSize: pageSize})
	if err != nil {
		return 0, err
	}
	rows := make([]*model.FeedInbox, 0, len(followersResp.Users))
	for _, user := range followersResp.Users {
		if user.Id > 0 {
			rows = append(rows, &model.FeedInbox{UserId: user.Id, AuthorId: event.AuthorId, PostId: event.PostId, CreatedAt: event.CreatedAt})
		}
	}
	return svcCtx.InboxModel.BatchInsertIgnore(ctx, rows)
}
