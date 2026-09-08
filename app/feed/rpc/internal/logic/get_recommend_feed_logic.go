package logic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"esx/app/feed/rpc/internal/model"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/app/recommend/rpc/recommendservice"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	if in == nil || l.svcCtx == nil || in.PageSize <= 0 || in.PageSize > maxFeedPageSize || strings.TrimSpace(in.RequestId) == "" || len(strings.TrimSpace(in.RequestId)) > 128 || len(strings.TrimSpace(in.Scene)) > 64 || len(strings.TrimSpace(in.SessionId)) > 128 || len(strings.TrimSpace(in.ExperimentId)) > 128 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if in.UserId < 0 || len(strings.TrimSpace(in.AnonymousId)) > 256 || (in.UserId == 0 && strings.TrimSpace(in.AnonymousId) == "") {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	requestID := strings.TrimSpace(in.RequestId)
	binding := fallbackRequestBinding(in)
	if stateID, expiresAt, matched, err := decodeFallbackCursor(
		l.svcCtx.Config.CursorSecret, in.Cursor, binding, l.now(),
	); err != nil {
		return nil, errx.NewWithCode(errx.ParamError)
	} else if matched {
		return l.continueFallback(in, binding, stateID, expiresAt)
	}

	scene := strings.TrimSpace(in.Scene)
	if scene == "" {
		scene = defaultRecommendScene
	}
	if l.svcCtx.RecommendService == nil {
		if in.Cursor != "" {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		l.Error("RecommendService is not configured; using rule fallback")
		return l.startFallback(in, binding)
	}
	recommendation, err := l.svcCtx.RecommendService.GetRecommendPosts(l.ctx, &recommendservice.GetRecommendPostsReq{
		UserId: in.UserId, AnonymousId: strings.TrimSpace(in.AnonymousId), Scene: scene,
		RequestId: requestID, SessionId: strings.TrimSpace(in.SessionId), Cursor: in.Cursor,
		PageSize: in.PageSize, ExperimentId: strings.TrimSpace(in.ExperimentId),
	})
	if err != nil {
		if in.Cursor != "" || !recommendationCanFallback(err) {
			return nil, errx.FromRPCError(err)
		}
		l.Errorw("RecommendService.GetRecommendPosts failed; using rule fallback", logx.Field("requestId", requestID), logx.Field("err", err.Error()))
		return l.startFallback(in, binding)
	}
	if recommendation == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if len(recommendation.Posts) == 0 {
		return &pb.GetRecommendFeedResp{Items: []*pb.FeedItem{}, RequestId: requestID, HasMore: recommendation.HasMore, NextCursor: recommendation.NextCursor}, nil
	}

	response, err := l.enrichRecommendation(in, recommendation)
	if err != nil {
		l.Errorw("recommendation enrichment failed",
			logx.Field("requestId", requestID), logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	return response, nil
}

func (l *GetRecommendFeedLogic) now() time.Time {
	if l.svcCtx.Now != nil {
		return l.svcCtx.Now()
	}
	return time.Now()
}

func fallbackRequestBinding(in *pb.GetRecommendFeedReq) model.FallbackBinding {
	identity := "a:" + strings.TrimSpace(in.AnonymousId)
	if in.UserId > 0 {
		identity = "u:" + strconv.FormatInt(in.UserId, 10)
	}
	digest := sha256.Sum256([]byte(identity))
	scene := strings.TrimSpace(in.Scene)
	if scene == "" {
		scene = defaultRecommendScene
	}
	return model.FallbackBinding{IdentityHash: hex.EncodeToString(digest[:]), RequestID: strings.TrimSpace(in.RequestId), Scene: scene, SessionID: strings.TrimSpace(in.SessionId), ExperimentID: strings.TrimSpace(in.ExperimentId), PageSize: in.PageSize}
}

func recommendationCanFallback(err error) bool {
	return errx.Is(err, errx.ServiceUnavailable) || status.Code(err) == codes.Unavailable || status.Code(err) == codes.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded)
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
