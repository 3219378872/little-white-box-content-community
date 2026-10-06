package feed

import (
	"context"
	"strings"

	"esx/app/feed/rpc/feedservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

type GetRecommendFeedLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRecommendFeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRecommendFeedLogic {
	return &GetRecommendFeedLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetRecommendFeed 获取推荐流：登录用户或匿名设备至少有一个身份；声明支持广告槽位时
// 并行查询广告，在推荐结果补全后再按槽位插入。
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
	// ADS-021：只在请求显式声明支持广告槽位时查询广告；未声明的客户端得到与当前一致的响应。
	var sponsored <-chan sponsoredResult
	if req.AdSlots == 1 {
		sponsored = startSponsored(l.ctx, l.svcCtx, sponsoredRequest{
			userID: userID, sessionID: strings.TrimSpace(req.SessionId), requestID: strings.TrimSpace(req.RequestId),
			cursor: req.Cursor, scene: scene, market: strings.ToUpper(strings.TrimSpace(req.Market)), pageSize: req.PageSize,
		})
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
			logging.Field("err", err.Error()),
			logging.Field("requestId", req.RequestId),
		)
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		l.Error("FeedService.GetRecommendFeed returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	// 批量补全作者资料与当前用户的点赞状态。
	enrichment, err := loadFeedEnrichment(l.ctx, l.svcCtx, result.Items, userID)
	if err != nil {
		l.Errorw("failed to enrich recommend feed", logging.Field("err", err.Error()))
		return nil, err
	}

	items := make([]types.RecommendFeedItem, 0, len(result.Items))
	for index, item := range result.Items {
		if item == nil {
			continue
		}
		items = append(items, recommendFeedItem(item, index, enrichment, req.ExperimentId))
	}
	// 下游未回传 requestId 时沿用客户端的值，保证曝光上报能与本次请求关联。
	requestID := result.RequestId
	if requestID == "" {
		requestID = req.RequestId
	}

	resp = &types.GetRecommendFeedResp{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
		RequestId:  requestID,
	}
	if sponsored != nil {
		resp.Sponsored = placeSponsored(items, (<-sponsored).slots)
	}
	return resp, nil
}

// recommendFeedItem 把 Feed 服务的条目与作者、点赞信息合并为公开响应项；
// 推荐元数据缺失时（规则兜底）填入固定默认值，客户端上报时字段始终非空。
func recommendFeedItem(item *feedservice.FeedItem, index int, enrichment *feedEnrichment, experimentID string) types.RecommendFeedItem {
	author := enrichment.author(item.AuthorId)
	return types.RecommendFeedItem{
		PostId:        item.PostId,
		AuthorId:      item.AuthorId,
		AuthorName:    author.Name,
		AuthorAvatar:  author.Avatar,
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
		ExperimentId:  defaultString(item.ExperimentId, experimentID),
		Position:      defaultPosition(item.Position, index),
	}
}

// defaultString 在值为空时返回兜底值。
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// defaultPosition 在下游未给出位置时按返回顺序从 1 编号。
func defaultPosition(position int32, index int) int32 {
	if position > 0 {
		return position
	}
	return int32(index + 1)
}
