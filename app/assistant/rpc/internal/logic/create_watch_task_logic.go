package logic

import (
	"context"
	"strings"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateWatchTaskLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateWatchTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateWatchTaskLogic {
	return &CreateWatchTaskLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CreateWatchTaskLogic) CreateWatchTask(in *pb.CreateWatchTaskReq) (*pb.CreateWatchTaskResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.ConditionType) == "" || strings.TrimSpace(in.TargetType) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	return nil, unavailableUntilStore()
}
