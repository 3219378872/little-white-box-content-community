package logic

import (
	"context"
	"math"
	"sort"

	"errx"
	"esx/app/content/contentservice"
	"esx/app/feed/internal/svc"
	"esx/app/feed/xiaobaihe/feed/pb"
	"user/userservice"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFollowFeedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetFollowFeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFollowFeedLogic {
	return &GetFollowFeedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetFollowFeedLogic) GetFollowFeed(in *pb.GetFollowFeedReq) (*pb.GetFollowFeedResp, error) {
	if in.UserId <= 0 || in.PageSize <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	cursorCreatedAt := in.CursorCreatedAt
	cursorPostID := in.CursorPostId
	if cursorCreatedAt <= 0 {
		cursorCreatedAt = math.MaxInt64
	}
	if cursorPostID <= 0 {
		cursorPostID = math.MaxInt64
	}
	limit := int64(in.PageSize) + 1

	inboxRows, err := l.svcCtx.InboxModel.FindByUserBefore(l.ctx, in.UserId, cursorCreatedAt, cursorPostID, limit)
	if err != nil {
		l.Errorw("InboxModel.FindByUserBefore failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	followingResp, err := l.svcCtx.UserService.GetFollowing(l.ctx, &userservice.GetFollowingReq{UserId: in.UserId, Page: 1, PageSize: 1000})
	if err != nil {
		l.Errorw("UserService.GetFollowing failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	authorIDs := make([]int64, 0, len(followingResp.Users))
	for _, user := range followingResp.Users {
		if user.Id > 0 {
			authorIDs = append(authorIDs, user.Id)
		}
	}

	outboxRows, err := l.svcCtx.OutboxModel.FindByAuthorsBefore(l.ctx, authorIDs, cursorCreatedAt, cursorPostID, limit)
	if err != nil {
		l.Errorw("OutboxModel.FindByAuthorsBefore failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	itemsByPostID := make(map[int64]*pb.FeedItem, len(inboxRows)+len(outboxRows))
	for _, row := range inboxRows {
		itemsByPostID[row.PostId] = &pb.FeedItem{PostId: row.PostId, AuthorId: row.AuthorId, CreatedAt: row.CreatedAt, FeedType: 1}
	}
	for _, row := range outboxRows {
		itemsByPostID[row.PostId] = &pb.FeedItem{PostId: row.PostId, AuthorId: row.AuthorId, CreatedAt: row.CreatedAt, FeedType: 1}
	}
	items := make([]*pb.FeedItem, 0, len(itemsByPostID))
	for _, item := range itemsByPostID {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt == items[j].CreatedAt {
			return items[i].PostId > items[j].PostId
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})

	postIDs := make([]int64, 0, len(items))
	for _, item := range items {
		postIDs = append(postIDs, item.PostId)
	}
	if len(postIDs) > 0 {
		if _, err := l.svcCtx.ContentService.GetPostsByIds(l.ctx, &contentservice.GetPostsByIdsReq{PostIds: postIDs}); err != nil {
			l.Errorw("ContentService.GetPostsByIds failed", logx.Field("err", err.Error()))
			return nil, errx.NewWithCode(errx.SystemError)
		}
	}

	hasMore := len(inboxRows)+len(outboxRows) > int(in.PageSize) || len(items) > int(in.PageSize)
	if hasMore {
		items = items[:in.PageSize]
	}
	resp := &pb.GetFollowFeedResp{Items: items, HasMore: hasMore}
	if len(items) > 0 {
		last := items[len(items)-1]
		resp.NextCursorCreatedAt = last.CreatedAt
		resp.NextCursorPostId = last.PostId
	}
	return resp, nil
}
