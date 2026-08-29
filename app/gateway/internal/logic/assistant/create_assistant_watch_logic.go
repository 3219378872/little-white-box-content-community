package assistant

import (
	"context"
	"strings"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateAssistantWatchLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateAssistantWatchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateAssistantWatchLogic {
	return &CreateAssistantWatchLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *CreateAssistantWatchLogic) CreateAssistantWatch(req *types.CreateAssistantWatchReq) (*types.CreateAssistantWatchResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if req == nil || strings.TrimSpace(req.ConditionType) == "" || strings.TrimSpace(req.TargetType) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	result, err := l.svcCtx.AssistantService.CreateWatchTask(l.ctx, &assistantservice.CreateWatchTaskReq{
		UserId:        userID,
		ConditionType: req.ConditionType,
		TargetType:    req.TargetType,
		TargetId:      req.TargetId,
		TargetText:    req.TargetText,
	})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	task := result.GetTask()
	if task == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &types.CreateAssistantWatchResp{Task: mapWatch(task)}, nil
}
