package logic

import (
	"context"
	"errors"
	"fmt"

	"errx"
	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type UnlikeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUnlikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnlikeLogic {
	return &UnlikeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UnlikeLogic) Unlike(in *pb.UnlikeReq) (*pb.UnlikeResp, error) {
	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if errors.Is(err, model.ErrNotFound) {
		return nil, errx.NewWithCode(errx.NotLikedYet)
	}
	if err != nil {
		l.Logger.Errorf("find like record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record.Status == model.StatusInactive {
		return nil, errx.NewWithCode(errx.NotLikedYet)
	}

	record.Status = model.StatusInactive
	if err := l.svcCtx.LikeRecordModel.Update(l.ctx, record); err != nil {
		l.Logger.Errorf("update like record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.decrLikeCount(in.TargetId, int64(in.TargetType)); err != nil {
		l.Logger.Errorf("decrease like count failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.UnlikeResp{}, nil
}

func (l *UnlikeLogic) decrLikeCount(targetID, targetType int64) error {
	if l.svcCtx.ActionCountModel == nil {
		return nil
	}

	if err := l.svcCtx.ActionCountModel.DecrLikeCount(l.ctx, targetID, targetType); err != nil {
		return err
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, targetID, targetType)
	if errors.Is(err, model.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	l.syncLikeCountCache(count)
	return nil
}

func (l *UnlikeLogic) syncLikeCountCache(count *model.ActionCount) {
	store := l.svcCtx.RedisStore
	if store == nil && l.svcCtx.Redis != nil {
		store = svc.NewRedisStore(l.svcCtx.Redis)
	}
	if store == nil {
		return
	}

	key := fmt.Sprintf("action_count:%d:%d", count.TargetId, count.TargetType)
	if err := store.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount)); err != nil {
		l.Logger.Errorf("sync like_count cache failed: %v", err)
	}
	if err := store.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount)); err != nil {
		l.Logger.Errorf("sync favorite_count cache failed: %v", err)
	}
	if err := store.Hset(key, "comment_count", fmt.Sprintf("%d", count.CommentCount)); err != nil {
		l.Logger.Errorf("sync comment_count cache failed: %v", err)
	}
	if err := store.Hset(key, "share_count", fmt.Sprintf("%d", count.ShareCount)); err != nil {
		l.Logger.Errorf("sync share_count cache failed: %v", err)
	}
	if err := store.Expire(key, model.CacheLongTTL); err != nil {
		l.Logger.Errorf("set cache expire failed: %v", err)
	}
}
