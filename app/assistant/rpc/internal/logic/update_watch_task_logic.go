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
	return &UpdateWatchTaskLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *UpdateWatchTaskLogic) UpdateWatchTask(in *pb.UpdateWatchTaskReq) (*pb.UpdateWatchTaskResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if in.Id <= 0 || in.ExpectedVersion <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx == nil || l.svcCtx.Watch == nil {
		return nil, unavailableUntilStore()
	}
	task, err := l.svcCtx.Watch.UpdateEnabled(l.ctx, in.UserId, in.Id, in.Enabled, in.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	return &pb.UpdateWatchTaskResp{Task: &pb.WatchTask{
		Id: task.ID, ConditionType: task.ConditionType, TargetType: task.TargetType,
		TargetId: task.TargetID, TargetText: task.TargetText, Enabled: task.Enabled,
		Version: task.Version, CreatedAt: task.CreatedAt,
	}}, nil
}
