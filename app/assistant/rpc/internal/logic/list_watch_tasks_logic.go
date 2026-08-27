package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWatchTasksLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListWatchTasksLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWatchTasksLogic {
	return &ListWatchTasksLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ListWatchTasksLogic) ListWatchTasks(in *pb.ListWatchTasksReq) (*pb.ListWatchTasksResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	return &pb.ListWatchTasksResp{Tasks: []*pb.WatchTask{}}, nil
}
