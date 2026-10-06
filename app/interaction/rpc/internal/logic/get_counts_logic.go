package logic

import (
	"context"
	"errors"
	model2 "esx/app/interaction/rpc/internal/model"
	svc2 "esx/app/interaction/rpc/internal/svc"
	pb "esx/kitex_gen/interaction"
	"fmt"
	"strconv"

	"esx/pkg/errx"

	"esx/pkg/logging"
)

// GetCountsLogic 承载 GetCounts 接口的业务逻辑；每个请求新建一个实例。
type GetCountsLogic struct {
	ctx    context.Context
	svcCtx *svc2.ServiceContext
	logging.Logger
}

// NewGetCountsLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetCountsLogic(ctx context.Context, svcCtx *svc2.ServiceContext) *GetCountsLogic {
	return &GetCountsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// GetCounts 读取目标的互动计数：先读缓存，未命中时经 singleflight 合并回源；
// 不存在的目标缓存短 TTL 的零值，避免反复穿透数据库。
func (l *GetCountsLogic) GetCounts(in *pb.GetCountsReq) (*pb.GetCountsResp, error) {
	key := actionCountCacheKey(in.TargetId, int64(in.TargetType))

	if resp, ok := l.readCountsFromCache(key); ok {
		return resp, nil
	}
	if l.svcCtx.ActionCountModel == nil {
		return &pb.GetCountsResp{}, nil
	}

	result, err, _ := l.svcCtx.SingleFlight.Do(key, func() (any, error) {
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

// actionCountCacheKey 是目标互动计数的缓存键。
func actionCountCacheKey(targetID, targetType int64) string {
	return fmt.Sprintf("interaction:action_count:%d:%d", targetID, targetType)
}

// invalidateActionCountCache 在计数变化后删除缓存，下次读取回源数据库。
func invalidateActionCountCache(ctx context.Context, svcCtx *svc2.ServiceContext, targetID, targetType int64) error {
	store := svcCtx.RedisStore
	if store == nil && svcCtx.Redis != nil {
		store = svc2.NewRedisStore(svcCtx.Redis)
	}
	if store == nil {
		return nil
	}
	return store.Del(ctx, actionCountCacheKey(targetID, targetType))
}

// readCountsFromCache 从缓存哈希读取计数；任一字段缺失或读取失败都按未命中处理。
func (l *GetCountsLogic) readCountsFromCache(key string) (*pb.GetCountsResp, bool) {
	store := l.redisStore()
	if store == nil {
		return nil, false
	}

	likeVal, err := store.Hget(l.ctx, key, "like_count")
	if err != nil {
		return nil, false
	}
	favoriteVal, err := store.Hget(l.ctx, key, "favorite_count")
	if err != nil {
		return nil, false
	}
	commentVal, err := store.Hget(l.ctx, key, "comment_count")
	if err != nil {
		return nil, false
	}

	return &pb.GetCountsResp{
		LikeCount:     parseInt64(likeVal, l.Logger),
		FavoriteCount: parseInt64(favoriteVal, l.Logger),
		CommentCount:  parseInt64(commentVal, l.Logger),
	}, true
}

// writeCountsToCache 把计数写入缓存哈希并设置 TTL；写入失败只记录，不影响响应。
func (l *GetCountsLogic) writeCountsToCache(key string, count *model2.ActionCount, ttlSeconds int) {
	store := l.redisStore()
	if store == nil {
		return
	}

	if err := store.Hset(l.ctx, key, "like_count", fmt.Sprintf("%d", count.LikeCount)); err != nil {
		l.Errorf("write like_count cache failed: %v", err)
	}
	if err := store.Hset(l.ctx, key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount)); err != nil {
		l.Errorf("write favorite_count cache failed: %v", err)
	}
	if err := store.Hset(l.ctx, key, "comment_count", fmt.Sprintf("%d", count.CommentCount)); err != nil {
		l.Errorf("write comment_count cache failed: %v", err)
	}
	if err := store.Hset(l.ctx, key, "share_count", fmt.Sprintf("%d", count.ShareCount)); err != nil {
		l.Errorf("write share_count cache failed: %v", err)
	}
	if err := store.Expire(l.ctx, key, ttlSeconds); err != nil {
		l.Errorf("set cache expire failed: %v", err)
	}
}

// redisStore 返回缓存存储；测试可注入 RedisStore，未配置 Redis 时返回 nil 表示不使用缓存。
func (l *GetCountsLogic) redisStore() svc2.RedisStore {
	if l.svcCtx.RedisStore != nil {
		return l.svcCtx.RedisStore
	}
	if l.svcCtx.Redis != nil {
		return svc2.NewRedisStore(l.svcCtx.Redis)
	}
	return nil
}

// parseInt64 解析缓存中的计数；值损坏时记录并按 0 处理。
func parseInt64(value string, logger logging.Logger) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		logger.Errorf("parseInt64 failed: value=%s, err=%v", value, err)
		return 0
	}
	return parsed
}
