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
	if record.Status == 0 {
		return nil, errx.NewWithCode(errx.NotLikedYet)
	}

	record.Status = 0
	if err := l.svcCtx.LikeRecordModel.Update(l.ctx, record); err != nil {
		l.Logger.Errorf("update like record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.decrLikeCount(in.TargetId, int64(in.TargetType)); err != nil {
		l.Logger.Errorf("decrease like count failed: %v", err)
	}

	return &pb.UnlikeResp{}, nil
}

func (l *UnlikeLogic) decrLikeCount(targetID, targetType int64) error {
	if l.svcCtx.ActionCountModel == nil {
		return nil
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, targetID, targetType)
	switch {
	case err == nil:
		if count.LikeCount > 0 {
			count.LikeCount--
		}
		if err := l.svcCtx.ActionCountModel.Update(l.ctx, count); err != nil {
			return err
		}
		l.syncLikeCountCache(count)
		return nil
	case errors.Is(err, model.ErrNotFound):
		return nil
	default:
		return err
	}
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
	_ = store.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount))
	_ = store.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount))
	_ = store.Hset(key, "comment_count", fmt.Sprintf("%d", count.CommentCount))
	_ = store.Hset(key, "share_count", fmt.Sprintf("%d", count.ShareCount))
	_ = store.Expire(key, 300)
}
