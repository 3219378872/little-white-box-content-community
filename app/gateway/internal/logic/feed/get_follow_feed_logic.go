// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package feed

import (
	"context"
	"errors"

	"errx"
	"esx/app/feed/rpc/feedservice"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFollowFeedLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取关注流
func NewGetFollowFeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFollowFeedLogic {
	return &GetFollowFeedLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetFollowFeedLogic) GetFollowFeed(req *types.GetFollowFeedReq) (resp *types.GetFollowFeedResp, err error) {
	if req == nil || req.PageSize <= 0 || req.PageSize > 100 || req.CursorCreatedAt < 0 || req.CursorPostId < 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil || userID <= 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}

	result, err := l.svcCtx.FeedService.GetFollowFeed(l.ctx, &feedservice.GetFollowFeedReq{
		UserId:          userID,
		CursorCreatedAt: req.CursorCreatedAt,
		CursorPostId:    req.CursorPostId,
		PageSize:        req.PageSize,
	})
	if err != nil {
		l.Errorw("FeedService.GetFollowFeed RPC failed", logx.Field("err", err.Error()))
		return nil, feedRPCError(err)
	}
	if result == nil {
		l.Error("FeedService.GetFollowFeed returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	enrichment, err := loadFeedEnrichment(l.ctx, l.svcCtx, result.Items, userID)
	if err != nil {
		l.Errorw("failed to enrich follow feed", logx.Field("err", err.Error()))
		return nil, err
	}

	items := make([]types.FeedItem, 0, len(result.Items))
	for _, item := range result.Items {
		if item == nil {
			continue
		}
		author := enrichment.author(item.AuthorId)
		items = append(items, types.FeedItem{
			PostId:        item.PostId,
			AuthorId:      item.AuthorId,
			AuthorName:    author.name,
			AuthorAvatar:  author.avatar,
			CreatedAt:     item.CreatedAt,
			FeedType:      item.FeedType,
			Title:         item.Title,
			Content:       item.Content,
			Images:        copyStrings(item.Images),
			Tags:          copyStrings(item.Tags),
			ViewCount:     item.ViewCount,
			LikeCount:     item.LikeCount,
			CommentCount:  item.CommentCount,
			FavoriteCount: item.FavoriteCount,
			IsLiked:       enrichment.isLiked(item.PostId),
		})
	}

	return &types.GetFollowFeedResp{
		Items:               items,
		HasMore:             result.HasMore,
		NextCursorCreatedAt: result.NextCursorCreatedAt,
		NextCursorPostId:    result.NextCursorPostId,
	}, nil
}

func feedRPCError(err error) error {
	var bizErr *errx.BizError
	if errors.As(err, &bizErr) {
		return bizErr
	}
	return errx.Wrap(err, errx.SystemError)
}
