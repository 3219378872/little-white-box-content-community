// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package feed

import (
	"context"
	"strings"

	"errx"
	"esx/app/feed/rpc/feedservice"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRecommendFeedLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取推荐流
func NewGetRecommendFeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRecommendFeedLogic {
	return &GetRecommendFeedLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRecommendFeedLogic) GetRecommendFeed(req *types.GetRecommendFeedReq) (resp *types.GetRecommendFeedResp, err error) {
	if req == nil || req.PageSize <= 0 || req.PageSize > 100 || strings.TrimSpace(req.RequestId) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	userID, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)
	if userID < 0 {
		userID = 0
	}
	anonymousID := strings.TrimSpace(req.AnonymousId)
	if userID == 0 && anonymousID == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	scene := strings.TrimSpace(req.Scene)
	if scene == "" {
		scene = "home"
	}

	result, err := l.svcCtx.FeedService.GetRecommendFeed(l.ctx, &feedservice.GetRecommendFeedReq{
		UserId:       userID,
		AnonymousId:  anonymousID,
		Scene:        scene,
		RequestId:    strings.TrimSpace(req.RequestId),
		SessionId:    strings.TrimSpace(req.SessionId),
		Cursor:       req.Cursor,
		PageSize:     req.PageSize,
		ExperimentId: strings.TrimSpace(req.ExperimentId),
	})
	if err != nil {
		l.Errorw("FeedService.GetRecommendFeed RPC failed",
			logx.Field("err", err.Error()),
			logx.Field("requestId", req.RequestId),
		)
		return nil, feedRPCError(err)
	}
	if result == nil {
		l.Error("FeedService.GetRecommendFeed returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	enrichment, err := loadFeedEnrichment(l.ctx, l.svcCtx, result.Items, userID)
	if err != nil {
		l.Errorw("failed to enrich recommend feed", logx.Field("err", err.Error()))
		return nil, err
	}

	items := make([]types.RecommendFeedItem, 0, len(result.Items))
	for index, item := range result.Items {
		if item == nil {
			continue
		}
		author := enrichment.author(item.AuthorId)
		items = append(items, types.RecommendFeedItem{
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
			Score:         item.Score,
			Reason:        defaultString(item.Reason, "fallback"),
			RecallSource:  defaultString(item.RecallSource, "feed"),
			ModelVersion:  defaultString(item.ModelVersion, "rule-fallback-v1"),
			ExperimentId:  defaultString(item.ExperimentId, req.ExperimentId),
			Position:      defaultPosition(item.Position, index),
		})
	}
	requestID := result.RequestId
	if requestID == "" {
		requestID = req.RequestId
	}

	return &types.GetRecommendFeedResp{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
		RequestId:  requestID,
	}, nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultPosition(position int32, index int) int32 {
	if position > 0 {
		return position
	}
	return int32(index + 1)
}
