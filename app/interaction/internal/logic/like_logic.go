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

type LikeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewLikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikeLogic {
	return &LikeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *LikeLogic) Like(in *pb.LikeReq) (*pb.LikeResp, error) {
	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		l.Logger.Errorf("find like record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record != nil && record.Status == 1 {
		return nil, errx.NewWithCode(errx.AlreadyLiked)
	}

	if record == nil {
		_, err = l.svcCtx.LikeRecordModel.Insert(l.ctx, &model.LikeRecord{
			UserId:     in.UserId,
			TargetId:   in.TargetId,
			TargetType: int64(in.TargetType),
			Status:     1,
		})
	} else {
		record.Status = 1
		err = l.svcCtx.LikeRecordModel.Update(l.ctx, record)
	}
	if err != nil {
		l.Logger.Errorf("persist like record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.incrLikeCount(in.TargetId, int64(in.TargetType)); err != nil {
		l.Logger.Errorf("increase like count failed: %v", err)
	}

	return &pb.LikeResp{}, nil
}

func (l *LikeLogic) incrLikeCount(targetID, targetType int64) error {
	if l.svcCtx.ActionCountModel == nil {
		return nil
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, targetID, targetType)
	switch {
	case err == nil:
		count.LikeCount++
		if err := l.svcCtx.ActionCountModel.Update(l.ctx, count); err != nil {
			return err
		}
		l.syncLikeCountCache(count)
		return nil
	case errors.Is(err, model.ErrNotFound):
		count = &model.ActionCount{
			TargetId:   targetID,
			TargetType: targetType,
			LikeCount:  1,
		}
		if _, err := l.svcCtx.ActionCountModel.Insert(l.ctx, count); err != nil {
			return err
		}
		l.syncLikeCountCache(count)
		return nil
	default:
		return err
	}
}

func (l *LikeLogic) syncLikeCountCache(count *model.ActionCount) {
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
