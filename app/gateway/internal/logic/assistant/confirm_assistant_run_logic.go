package assistant

import (
	"context"
	"strings"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// ConfirmAssistantRunLogic 承载 ConfirmAssistantRun 接口的业务逻辑；每个请求新建一个实例。
type ConfirmAssistantRunLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewConfirmAssistantRunLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewConfirmAssistantRunLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfirmAssistantRunLogic {
	return &ConfirmAssistantRunLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// ConfirmAssistantRun 回应运行中等待用户确认的工具调用（批准或拒绝）。
func (l *ConfirmAssistantRunLogic) ConfirmAssistantRun(req *types.ConfirmAssistantRunReq) (*types.ConfirmAssistantRunResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Id <= 0 || strings.TrimSpace(req.CallId) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if _, err := l.svcCtx.AssistantService.ConfirmRunTool(l.ctx, &assistantservice.ConfirmRunToolReq{
		UserId: userID, RunId: req.Id, CallId: req.CallId, Approved: req.Approved,
	}); err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.ConfirmAssistantRunResp{}, nil
}
