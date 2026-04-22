package logic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"errx"
	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type FavoriteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FavoriteLogic {
	return &FavoriteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FavoriteLogic) Favorite(in *pb.FavoriteReq) (*pb.FavoriteResp, error) {
	record, err := l.svcCtx.FavoriteModel.FindOneByUserIdPostId(l.ctx, in.UserId, in.PostId)
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		l.Logger.Errorf("find favorite record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record != nil && record.Status == model.StatusActive {
		return nil, errx.NewWithCode(errx.AlreadyFavorited)
	}

	if record == nil {
		_, err = l.svcCtx.FavoriteModel.Insert(l.ctx, &model.Favorite{
			UserId:   in.UserId,
			PostId:   in.PostId,
			FolderId: sql.NullInt64{},
			Status:   model.StatusActive,
		})
	} else {
		record.Status = model.StatusActive
		err = l.svcCtx.FavoriteModel.Update(l.ctx, record)
	}
	if err != nil {
		l.Logger.Errorf("persist favorite record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.incrFavoriteCount(in.PostId); err != nil {
		l.Logger.Errorf("increase favorite count failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.FavoriteResp{}, nil
}

func (l *FavoriteLogic) incrFavoriteCount(postID int64) error {
	if l.svcCtx.ActionCountModel == nil {
		return nil
	}

	if err := l.svcCtx.ActionCountModel.IncrFavoriteCount(l.ctx, postID, 1); err != nil {
		return err
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, postID, 1)
	if err != nil {
		return err
	}
	l.syncFavoriteCountCache(count)
	return nil
}

func (l *FavoriteLogic) syncFavoriteCountCache(count *model.ActionCount) {
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
