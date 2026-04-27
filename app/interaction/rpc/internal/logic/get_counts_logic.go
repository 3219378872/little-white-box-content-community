package logic

import (
	"context"
	"errors"
	model2 "esx/app/interaction/rpc/internal/model"
	svc2 "esx/app/interaction/rpc/internal/svc"
	"esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"
	"fmt"
	"strconv"

	"errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCountsLogic struct {
	ctx    context.Context
	svcCtx *svc2.ServiceContext
	logx.Logger
}

func NewGetCountsLogic(ctx context.Context, svcCtx *svc2.ServiceContext) *GetCountsLogic {
	return &GetCountsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetCountsLogic) GetCounts(in *pb.GetCountsReq) (*pb.GetCountsResp, error) {
	key := fmt.Sprintf("interaction:action_count:%d:%d", in.TargetId, in.TargetType)

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
			if errors.Is(err, model2.ErrNotFound) {
				resp := &pb.GetCountsResp{}
				l.writeCountsToCache(key, &model2.ActionCount{TargetId: in.TargetId, TargetType: int64(in.TargetType)}, model2.CacheShortTTL)
				return resp, nil
			}
			return nil, err
		}

		resp := &pb.GetCountsResp{
			LikeCount:     count.LikeCount,
			FavoriteCount: count.FavoriteCount,
			CommentCount:  count.CommentCount,
		}
		l.writeCountsToCache(key, count, model2.CacheLongTTL)
		return resp, nil
	})
	if err != nil {
		l.Errorf("get counts failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
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

func (l *GetCountsLogic) writeCountsToCache(key string, count *model2.ActionCount, ttlSeconds int) {
	store := l.redisStore()
	if store == nil {
		return
	}

	if err := store.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount)); err != nil {
		l.Errorf("write like_count cache failed: %v", err)
	}
	if err := store.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount)); err != nil {
		l.Errorf("write favorite_count cache failed: %v", err)
	}
	if err := store.Hset(key, "comment_count", fmt.Sprintf("%d", count.CommentCount)); err != nil {
		l.Errorf("write comment_count cache failed: %v", err)
	}
	if err := store.Hset(key, "share_count", fmt.Sprintf("%d", count.ShareCount)); err != nil {
		l.Errorf("write share_count cache failed: %v", err)
	}
	if err := store.Expire(key, ttlSeconds); err != nil {
		l.Errorf("set cache expire failed: %v", err)
	}
}

func (l *GetCountsLogic) redisStore() svc2.RedisStore {
	if l.svcCtx.RedisStore != nil {
		return l.svcCtx.RedisStore
	}
	if l.svcCtx.Redis != nil {
		return svc2.NewRedisStore(l.svcCtx.Redis)
	}
	return nil
}

func parseInt64(value string) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		logx.Errorf("parseInt64 failed: value=%s, err=%v", value, err)
		return 0
	}
	return parsed
}
