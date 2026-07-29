package logic

import (
	"context"
	"math"
	"sort"

	"errx"
	"esx/app/feed/rpc/internal/model"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"
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
	if in == nil || in.UserId <= 0 || in.PageSize <= 0 || in.PageSize > maxFeedPageSize {
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
	if followingResp == nil {
		l.Error("UserService.GetFollowing returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	authorIDs := make([]int64, 0, len(followingResp.Users))
	for _, user := range followingResp.Users {
		if user != nil && user.Id > 0 {
			authorIDs = append(authorIDs, user.Id)
		}
	}

	var outboxRows []*model.FeedOutbox
	if len(authorIDs) > 0 {
		outboxRows, err = l.svcCtx.OutboxModel.FindByAuthorsBefore(l.ctx, authorIDs, cursorCreatedAt, cursorPostID, limit)
		if err != nil {
			l.Errorw("OutboxModel.FindByAuthorsBefore failed", logx.Field("err", err.Error()))
			return nil, errx.NewWithCode(errx.SystemError)
		}
	}

	itemsByPostID := make(map[int64]*pb.FeedItem, len(inboxRows)+len(outboxRows))
	for _, row := range inboxRows {
		if row == nil || row.PostId <= 0 {
			continue
		}
		itemsByPostID[row.PostId] = &pb.FeedItem{PostId: row.PostId, AuthorId: row.AuthorId, CreatedAt: row.CreatedAt, FeedType: feedTypeFollow}
	}
	for _, row := range outboxRows {
		if row == nil || row.PostId <= 0 {
			continue
		}
		candidate := &pb.FeedItem{PostId: row.PostId, AuthorId: row.AuthorId, CreatedAt: row.CreatedAt, FeedType: feedTypeFollow}
		if existing := itemsByPostID[row.PostId]; existing == nil || candidate.CreatedAt > existing.CreatedAt {
			itemsByPostID[row.PostId] = candidate
		}
	}
	rawItems := make([]*pb.FeedItem, 0, len(itemsByPostID))
	for _, item := range itemsByPostID {
		rawItems = append(rawItems, item)
	}
	sort.Slice(rawItems, func(i, j int) bool {
		if rawItems[i].CreatedAt == rawItems[j].CreatedAt {
			return rawItems[i].PostId > rawItems[j].PostId
		}
		return rawItems[i].CreatedAt > rawItems[j].CreatedAt
	})

	rendered, err := enrichFeedItems(l.ctx, l.svcCtx.ContentService, rawItems)
	if err != nil {
		l.Errorw("ContentService.GetPostsByIds failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	renderedByPostID := make(map[int64]*pb.FeedItem, len(rendered))
	for _, item := range rendered {
		renderedByPostID[item.PostId] = item
	}

	items := make([]*pb.FeedItem, 0, in.PageSize)
	var lastScanned *pb.FeedItem
	hasUnscanned := false
	for _, rawItem := range rawItems {
		if len(items) == int(in.PageSize) {
			hasUnscanned = true
			break
		}
		lastScanned = rawItem
		if item := renderedByPostID[rawItem.PostId]; item != nil {
			items = append(items, item)
		}
	}
	hasMore := hasUnscanned || len(inboxRows) == int(limit) || len(outboxRows) == int(limit)
	resp := &pb.GetFollowFeedResp{Items: items, HasMore: hasMore}
	if lastScanned != nil {
		resp.NextCursorCreatedAt = lastScanned.CreatedAt
		resp.NextCursorPostId = lastScanned.PostId
	}
	return resp, nil
}
