package assistant

import (
	"context"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListAssistantWatchLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAssistantWatchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAssistantWatchLogic {
	return &ListAssistantWatchLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ListAssistantWatchLogic) ListAssistantWatch(_ *types.ListAssistantWatchReq) (*types.ListAssistantWatchResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	result, err := l.svcCtx.AssistantService.ListWatchTasks(l.ctx, &assistantservice.ListWatchTasksReq{UserId: userID})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	tasks := make([]types.AssistantWatchTask, 0, len(result.GetTasks()))
	for _, task := range result.GetTasks() {
		if task == nil {
			continue
		}
		tasks = append(tasks, mapWatch(task))
	}
	return &types.ListAssistantWatchResp{Tasks: tasks}, nil
}
