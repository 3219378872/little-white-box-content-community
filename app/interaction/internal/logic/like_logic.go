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
	if record != nil && record.Status == model.StatusActive {
		return nil, errx.NewWithCode(errx.AlreadyLiked)
	}

	if record == nil {
		_, err = l.svcCtx.LikeRecordModel.Insert(l.ctx, &model.LikeRecord{
			UserId:     in.UserId,
			TargetId:   in.TargetId,
			TargetType: int64(in.TargetType),
			Status:     model.StatusActive,
		})
	} else {
		record.Status = model.StatusActive
		err = l.svcCtx.LikeRecordModel.Update(l.ctx, record)
	}
	if err != nil {
		l.Logger.Errorf("persist like record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.incrLikeCount(in.TargetId, int64(in.TargetType)); err != nil {
		l.Logger.Errorf("increase like count failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.LikeResp{}, nil
}

func (l *LikeLogic) incrLikeCount(targetID, targetType int64) error {
	if l.svcCtx.ActionCountModel == nil {
		return nil
	}

	if err := l.svcCtx.ActionCountModel.IncrLikeCount(l.ctx, targetID, targetType); err != nil {
		return err
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, targetID, targetType)
	if err != nil {
		return err
	}
	l.syncLikeCountCache(count)
	return nil
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
