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

const likeListTargetTypePost int64 = 1

// GetLikeListLogic 承载 GetLikeList 接口的业务逻辑；每个请求新建一个实例。
type GetLikeListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewGetLikeListLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetLikeListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLikeListLogic {
	return &GetLikeListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// GetLikeList 分页返回用户当前点赞的帖子 ID，按点赞时间倒序；超出分页窗口视为参数错误。
func (l *GetLikeListLogic) GetLikeList(in *pb.GetLikeListReq) (*pb.GetLikeListResp, error) {
	page := pageutil.ClampPage(in.Page)
	pageSize := pageutil.ClampPageSizeTo(in.PageSize, pageutil.DefaultPageSize, pageutil.InteractionMaxPageSize)

	postIDs, total, err := l.svcCtx.LikeRecordModel.FindActiveTargetIds(l.ctx, in.UserId, likeListTargetTypePost, page, pageSize)
	if err != nil {
		if errors.Is(err, pageutil.ErrPageWindow) {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		l.Errorf("get like list failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.GetLikeListResp{
		PostIds: postIDs,
		Total:   total,
	}, nil
}
