package logic

import (
	"context"

	"esx/app/interaction/rpc/internal/svc"
	"esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

const likeListTargetTypePost int64 = 1

type GetLikeListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetLikeListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLikeListLogic {
	return &GetLikeListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetLikeListLogic) GetLikeList(in *pb.GetLikeListReq) (*pb.GetLikeListResp, error) {
	if in.Page < 1 {
		in.Page = 1
	}
	if in.PageSize < 1 || in.PageSize > 100 {
		in.PageSize = 20
	}

	postIDs, total, err := l.svcCtx.LikeRecordModel.FindActiveTargetIds(l.ctx, in.UserId, likeListTargetTypePost, in.Page, in.PageSize)
	if err != nil {
		l.Errorf("get like list failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.GetLikeListResp{
		PostIds: postIDs,
		Total:   total,
	}, nil
}
