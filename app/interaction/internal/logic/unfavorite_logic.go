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

type UnfavoriteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUnfavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnfavoriteLogic {
	return &UnfavoriteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UnfavoriteLogic) Unfavorite(in *pb.UnfavoriteReq) (*pb.UnfavoriteResp, error) {
	record, err := l.svcCtx.FavoriteModel.FindOneByUserIdPostId(l.ctx, in.UserId, in.PostId)
	if errors.Is(err, model.ErrNotFound) {
		return nil, errx.NewWithCode(errx.NotFavoritedYet)
	}
	if err != nil {
		l.Logger.Errorf("find favorite record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record.Status == 0 {
		return nil, errx.NewWithCode(errx.NotFavoritedYet)
	}

	record.Status = 0
	if err := l.svcCtx.FavoriteModel.Update(l.ctx, record); err != nil {
		l.Logger.Errorf("update favorite record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.decrFavoriteCount(in.PostId); err != nil {
		l.Logger.Errorf("decrease favorite count failed: %v", err)
	}

	return &pb.UnfavoriteResp{}, nil
}

func (l *UnfavoriteLogic) decrFavoriteCount(postID int64) error {
	if l.svcCtx.ActionCountModel == nil {
		return nil
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, postID, 1)
	switch {
	case err == nil:
		if count.FavoriteCount > 0 {
			count.FavoriteCount--
		}
		if err := l.svcCtx.ActionCountModel.Update(l.ctx, count); err != nil {
			return err
		}
		l.syncFavoriteCountCache(count)
		return nil
	case errors.Is(err, model.ErrNotFound):
		return nil
	default:
		return err
	}
}

func (l *UnfavoriteLogic) syncFavoriteCountCache(count *model.ActionCount) {
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
