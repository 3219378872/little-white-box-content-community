package logic

import (
	"context"
	"strings"
	"time"

	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/app/recommend/rpc/recommendservice"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

const defaultRecommendScene = "home"

type GetRecommendFeedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetRecommendFeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRecommendFeedLogic {
	return &GetRecommendFeedLogic{
		ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx),
	}
}

func (l *GetRecommendFeedLogic) GetRecommendFeed(in *pb.GetRecommendFeedReq) (*pb.GetRecommendFeedResp, error) {
	if in == nil || in.PageSize <= 0 || in.PageSize > maxFeedPageSize || strings.TrimSpace(in.RequestId) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if in.UserId <= 0 && strings.TrimSpace(in.AnonymousId) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	requestID := strings.TrimSpace(in.RequestId)
	if page, hotCursor, latestCursor, matched, err := decodeFallbackCursor(
		l.svcCtx.Config.CursorSecret, in.Cursor, requestID, time.Now(),
	); err != nil {
		return nil, errx.NewWithCode(errx.ParamError)
	} else if matched {
		return l.fallbackFeed(in, page, hotCursor, latestCursor)
	}

	scene := strings.TrimSpace(in.Scene)
	if scene == "" {
		scene = defaultRecommendScene
	}
	if l.svcCtx.RecommendService == nil {
		l.Error("RecommendService is not configured; using rule fallback")
		return l.fallbackFeed(in, 1, "", "")
	}
	recommendation, err := l.svcCtx.RecommendService.GetRecommendPosts(l.ctx, &recommendservice.GetRecommendPostsReq{
		UserId: in.UserId, AnonymousId: strings.TrimSpace(in.AnonymousId), Scene: scene,
		RequestId: requestID, SessionId: strings.TrimSpace(in.SessionId), Cursor: in.Cursor,
		PageSize: in.PageSize, ExperimentId: strings.TrimSpace(in.ExperimentId),
	})
	if err != nil || recommendation == nil || len(recommendation.Posts) == 0 {
		if err != nil {
			l.Errorw("RecommendService.GetRecommendPosts failed; using rule fallback",
				logx.Field("requestId", requestID), logx.Field("err", err.Error()))
		}
		return l.fallbackFeed(in, 1, "", "")
	}

	response, err := l.enrichRecommendation(in, recommendation)
	if err != nil {
		l.Errorw("recommendation enrichment failed; using rule fallback",
			logx.Field("requestId", requestID), logx.Field("err", err.Error()))
		return l.fallbackFeed(in, 1, "", "")
	}
	if len(response.Items) == 0 {
		return l.fallbackFeed(in, 1, "", "")
	}
	return response, nil
}

func (l *GetRecommendFeedLogic) enrichRecommendation(
	in *pb.GetRecommendFeedReq,
	recommendation *recommendservice.GetRecommendPostsResp,
) (*pb.GetRecommendFeedResp, error) {
	if l.svcCtx.ContentService == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	baseItems := make([]*pb.FeedItem, 0, len(recommendation.Posts))
	for index, candidate := range recommendation.Posts {
		if candidate == nil || candidate.PostId <= 0 {
			continue
		}
		position := candidate.Position
		if position <= 0 {
			position = int32(index + 1)
		}
		experimentID := candidate.ExperimentId
		if experimentID == "" {
			experimentID = in.ExperimentId
		}
		baseItems = append(baseItems, &pb.FeedItem{
			PostId: candidate.PostId, FeedType: feedTypeRecommend,
			Score: candidate.Score, Reason: candidate.Reason,
			RecallSource: candidate.RecallSource, ModelVersion: candidate.ModelVersion,
			ExperimentId: experimentID, Position: position,
		})
	}
	items, err := enrichFeedItems(l.ctx, l.svcCtx.ContentService, baseItems)
	if err != nil {
		return nil, err
	}
	requestID := recommendation.RequestId
	if requestID == "" {
		requestID = in.RequestId
	}
	return &pb.GetRecommendFeedResp{
		Items: items, HasMore: recommendation.HasMore,
		NextCursor: recommendation.NextCursor, RequestId: requestID,
	}, nil
}

type fallbackSource struct {
	items []*pb.FeedItem
	index int
}

func (l *GetRecommendFeedLogic) fallbackFeed(in *pb.GetRecommendFeedReq, page int32, hotCursor, latestCursor string) (*pb.GetRecommendFeedResp, error) {
	if l.svcCtx.ContentService == nil || page <= 0 {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	hot, hotErr := l.svcCtx.ContentService.GetPostList(l.ctx, &contentservice.GetPostListReq{
		PageSize: in.PageSize, SortBy: 3, Cursor: hotCursor,
	})
	latest, latestErr := l.svcCtx.ContentService.GetPostList(l.ctx, &contentservice.GetPostListReq{
		PageSize: in.PageSize, SortBy: 1, Cursor: latestCursor,
	})
	if hotErr != nil && latestErr != nil {
		l.Errorw("all feed fallback sources failed",
			logx.Field("hotErr", hotErr.Error()), logx.Field("latestErr", latestErr.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	sources := make([]fallbackSource, 0, 3)
	hasMore := false
	if page == 1 && in.UserId > 0 && l.svcCtx.InboxModel != nil && l.svcCtx.OutboxModel != nil && l.svcCtx.UserService != nil {
		follow, err := NewGetFollowFeedLogic(l.ctx, l.svcCtx).GetFollowFeed(&pb.GetFollowFeedReq{
			UserId: in.UserId, PageSize: in.PageSize,
		})
		if err == nil && follow != nil {
			for _, item := range follow.Items {
				if item == nil {
					continue
				}
				item.Reason = "follow fallback"
				item.RecallSource = "follow"
				item.ModelVersion = "rule-fallback-v2"
				item.ExperimentId = in.ExperimentId
			}
			sources = append(sources, fallbackSource{items: follow.Items})
			hasMore = hasMore || follow.HasMore
		}
	}
	nextHotCursor, nextLatestCursor := "", ""
	if hotErr == nil && hot != nil {
		sources = append(sources, fallbackSource{items: fallbackItems(hot.Posts, "popular")})
		nextHotCursor = hot.NextCursor
		hasMore = hasMore || nextHotCursor != ""
	}
	if latestErr == nil && latest != nil {
		sources = append(sources, fallbackSource{items: fallbackItems(latest.Posts, "latest")})
		nextLatestCursor = latest.NextCursor
		hasMore = hasMore || nextLatestCursor != ""
	}

	items := interleaveFallbackSources(sources, int(in.PageSize), in.ExperimentId)
	nextCursor := ""
	if hasMore {
		var err error
		nextCursor, err = encodeFallbackCursor(l.svcCtx.Config.CursorSecret, in.RequestId, page+1, nextHotCursor, nextLatestCursor, time.Now())
		if err != nil {
			l.Errorw("encode feed fallback cursor failed", logx.Field("err", err.Error()))
			return nil, errx.NewWithCode(errx.SystemError)
		}
	}
	return &pb.GetRecommendFeedResp{
		Items: items, HasMore: hasMore, NextCursor: nextCursor, RequestId: in.RequestId,
	}, nil
}

func fallbackItems(posts []*contentservice.PostInfo, source string) []*pb.FeedItem {
	items := make([]*pb.FeedItem, 0, len(posts))
	for _, post := range posts {
		if item := fallbackFeedItem(post, source); item != nil {
			items = append(items, item)
		}
	}
	return items
}

func interleaveFallbackSources(sources []fallbackSource, limit int, experimentID string) []*pb.FeedItem {
	items := make([]*pb.FeedItem, 0, limit)
	seen := make(map[int64]struct{}, limit)
	for len(items) < limit {
		added := false
		for i := range sources {
			for sources[i].index < len(sources[i].items) {
				item := sources[i].items[sources[i].index]
				sources[i].index++
				if item == nil {
					continue
				}
				if _, exists := seen[item.PostId]; exists {
					continue
				}
				seen[item.PostId] = struct{}{}
				item.Position = int32(len(items) + 1)
				if item.ExperimentId == "" {
					item.ExperimentId = experimentID
				}
				items = append(items, item)
				added = true
				break
			}
			if len(items) == limit {
				break
			}
		}
		if !added {
			break
		}
	}
	return items
}
