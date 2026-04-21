package logic

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCountsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetCountsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCountsLogic {
	return &GetCountsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetCountsLogic) GetCounts(in *pb.GetCountsReq) (*pb.GetCountsResp, error) {
	key := fmt.Sprintf("action_count:%d:%d", in.TargetId, in.TargetType)

	if resp, ok := l.readCountsFromCache(key); ok {
		return resp, nil
	}
	if l.svcCtx.ActionCountModel == nil {
		return &pb.GetCountsResp{}, nil
	}

	result, err, _ := l.svcCtx.SingleFlight.Do(key, func() (interface{}, error) {
		if resp, ok := l.readCountsFromCache(key); ok {
			return resp, nil
		}

		count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, in.TargetId, int64(in.TargetType))
		if err != nil {
			if errors.Is(err, model.ErrNotFound) {
				resp := &pb.GetCountsResp{}
				l.writeCountsToCache(key, &model.ActionCount{TargetId: in.TargetId, TargetType: int64(in.TargetType)}, 30)
				return resp, nil
			}
			return nil, err
		}

		resp := &pb.GetCountsResp{
			LikeCount:     count.LikeCount,
			FavoriteCount: count.FavoriteCount,
			CommentCount:  count.CommentCount,
		}
		l.writeCountsToCache(key, count, 300)
		return resp, nil
	})
	if err != nil {
		l.Logger.Errorf("get counts failed: %v", err)
		return nil, err
	}

	return result.(*pb.GetCountsResp), nil
}

func (l *GetCountsLogic) readCountsFromCache(key string) (*pb.GetCountsResp, bool) {
	store := l.redisStore()
	if store == nil {
		return nil, false
	}

	likeVal, err := store.Hget(key, "like_count")
	if err != nil {
		return nil, false
	}
	favoriteVal, err := store.Hget(key, "favorite_count")
	if err != nil {
		return nil, false
	}
	commentVal, err := store.Hget(key, "comment_count")
	if err != nil {
		return nil, false
	}

	return &pb.GetCountsResp{
		LikeCount:     parseInt64(likeVal),
		FavoriteCount: parseInt64(favoriteVal),
		CommentCount:  parseInt64(commentVal),
	}, true
}

func (l *GetCountsLogic) writeCountsToCache(key string, count *model.ActionCount, ttlSeconds int) {
	store := l.redisStore()
	if store == nil {
		return
	}

	_ = store.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount))
	_ = store.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount))
	_ = store.Hset(key, "comment_count", fmt.Sprintf("%d", count.CommentCount))
	_ = store.Hset(key, "share_count", fmt.Sprintf("%d", count.ShareCount))
	_ = store.Expire(key, ttlSeconds)
}

func (l *GetCountsLogic) redisStore() svc.RedisStore {
	if l.svcCtx.RedisStore != nil {
		return l.svcCtx.RedisStore
	}
	if l.svcCtx.Redis != nil {
		return svc.NewRedisStore(l.svcCtx.Redis)
	}
	return nil
}

func parseInt64(value string) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
