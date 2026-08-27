package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/agent"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/assistant/watch"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateWatchTaskLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateWatchTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateWatchTaskLogic {
	return &CreateWatchTaskLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *CreateWatchTaskLogic) CreateWatchTask(in *pb.CreateWatchTaskReq) (*pb.CreateWatchTaskResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Watch == nil {
		return nil, unavailableUntilStore()
	}
	task := watch.Task{
		UserID: in.UserId, ConditionType: in.ConditionType, TargetType: in.TargetType,
		TargetID: in.TargetId, TargetText: in.TargetText,
	}
	if err := agent.WatchLookups(agent.Clients{
		Search:  l.svcCtx.SearchService,
		Content: l.svcCtx.ContentService,
		User:    l.svcCtx.UserService,
	}).Validate(l.ctx, task); err != nil {
		return nil, err
	}
	created, err := l.svcCtx.Watch.Create(l.ctx, task)
	if err != nil {
		return nil, err
	}
	task = created
	return &pb.CreateWatchTaskResp{Task: &pb.WatchTask{
		Id: task.ID, ConditionType: task.ConditionType, TargetType: task.TargetType,
		TargetId: task.TargetID, TargetText: task.TargetText, Enabled: task.Enabled, CreatedAt: task.CreatedAt,
	}}, nil
}
