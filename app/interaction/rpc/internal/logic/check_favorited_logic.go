package logic

import (
	"context"
	"errors"
	"esx/app/interaction/rpc/internal/model"
	"esx/app/interaction/rpc/internal/svc"
	pb "esx/kitex_gen/interaction"

	"esx/pkg/errx"

	"esx/pkg/logging"
)

// CheckFavoritedLogic 承载 CheckFavorited 接口的业务逻辑；每个请求新建一个实例。
type CheckFavoritedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewCheckFavoritedLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewCheckFavoritedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckFavoritedLogic {
	return &CheckFavoritedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// CheckFavorited 查询用户当前是否收藏了该帖子；取消收藏的记录视为未收藏。
func (l *CheckFavoritedLogic) CheckFavorited(in *pb.CheckFavoritedReq) (*pb.CheckFavoritedResp, error) {
	record, err := l.svcCtx.FavoriteModel.FindOneByUserIdPostId(l.ctx, in.UserId, in.PostId)
	if errors.Is(err, model.ErrNotFound) {
		return &pb.CheckFavoritedResp{IsFavorited: false}, nil
	}
	if err != nil {
		l.Errorf("check favorited failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.CheckFavoritedResp{IsFavorited: record.Status == model.StatusActive}, nil
}
