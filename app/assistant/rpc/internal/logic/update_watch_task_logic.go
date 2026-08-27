package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateWatchTaskLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateWatchTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateWatchTaskLogic {
	return &UpdateWatchTaskLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UpdateWatchTaskLogic) UpdateWatchTask(in *pb.UpdateWatchTaskReq) (*pb.UpdateWatchTaskResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if in.Id <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	return nil, unavailableUntilStore()
}
