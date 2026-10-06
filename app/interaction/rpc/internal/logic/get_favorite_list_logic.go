package logic

import (
	"context"
	"errors"
	"esx/app/interaction/rpc/internal/svc"
	pb "esx/kitex_gen/interaction"

	"esx/pkg/errx"
	"esx/pkg/pageutil"

	"esx/pkg/logging"
)

// GetFavoriteListLogic 承载 GetFavoriteList 接口的业务逻辑；每个请求新建一个实例。
type GetFavoriteListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewGetFavoriteListLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetFavoriteListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFavoriteListLogic {
	return &GetFavoriteListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// GetFavoriteList 分页返回用户当前收藏的帖子 ID，按收藏时间倒序；超出分页窗口视为参数错误。
func (l *GetFavoriteListLogic) GetFavoriteList(in *pb.GetFavoriteListReq) (*pb.GetFavoriteListResp, error) {
	page := pageutil.ClampPage(in.Page)
	pageSize := pageutil.ClampPageSizeTo(in.PageSize, pageutil.DefaultPageSize, pageutil.InteractionMaxPageSize)

	postIDs, total, err := l.svcCtx.FavoriteModel.FindActivePostIds(l.ctx, in.UserId, page, pageSize)
	if err != nil {
		if errors.Is(err, pageutil.ErrPageWindow) {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		l.Errorf("get favorite list failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.GetFavoriteListResp{
		PostIds: postIDs,
		Total:   total,
	}, nil
}
