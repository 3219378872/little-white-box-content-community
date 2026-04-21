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
	if record != nil && record.Status == 1 {
		return nil, errx.NewWithCode(errx.AlreadyFavorited)
	}

	if record == nil {
		_, err = l.svcCtx.FavoriteModel.Insert(l.ctx, &model.Favorite{
			UserId:   in.UserId,
			PostId:   in.PostId,
			FolderId: sql.NullInt64{},
			Status:   1,
		})
	} else {
		record.Status = 1
		err = l.svcCtx.FavoriteModel.Update(l.ctx, record)
	}
	if err != nil {
		l.Logger.Errorf("persist favorite record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.incrFavoriteCount(in.PostId); err != nil {
		l.Logger.Errorf("increase favorite count failed: %v", err)
	}

	return &pb.FavoriteResp{}, nil
}

func (l *FavoriteLogic) incrFavoriteCount(postID int64) error {
	if l.svcCtx.ActionCountModel == nil {
		return nil
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, postID, 1)
	switch {
	case err == nil:
		count.FavoriteCount++
		if err := l.svcCtx.ActionCountModel.Update(l.ctx, count); err != nil {
			return err
		}
		l.syncFavoriteCountCache(count)
		return nil
	case errors.Is(err, model.ErrNotFound):
		count = &model.ActionCount{
			TargetId:      postID,
			TargetType:    1,
			FavoriteCount: 1,
		}
		if _, err := l.svcCtx.ActionCountModel.Insert(l.ctx, count); err != nil {
			return err
		}
		l.syncFavoriteCountCache(count)
		return nil
	default:
		return err
	}
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
	_ = store.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount))
	_ = store.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount))
	_ = store.Hset(key, "comment_count", fmt.Sprintf("%d", count.CommentCount))
	_ = store.Hset(key, "share_count", fmt.Sprintf("%d", count.ShareCount))
	_ = store.Expire(key, 300)
}
