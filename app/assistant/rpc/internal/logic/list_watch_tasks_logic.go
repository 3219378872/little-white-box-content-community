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
	return &ListWatchTasksLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ListWatchTasksLogic) ListWatchTasks(in *pb.ListWatchTasksReq) (*pb.ListWatchTasksResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Watch == nil {
		return nil, unavailableUntilStore()
	}
	tasks, err := l.svcCtx.Watch.ListTasks(l.ctx, in.UserId)
	if err != nil {
		return nil, err
	}
	out := make([]*pb.WatchTask, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, &pb.WatchTask{
			Id: task.ID, ConditionType: task.ConditionType, TargetType: task.TargetType,
			TargetId: task.TargetID, TargetText: task.TargetText, Enabled: task.Enabled, CreatedAt: task.CreatedAt,
		})
	}
	return &pb.ListWatchTasksResp{Tasks: out}, nil
}
