// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

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

type ConfirmAssistantToolLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// Agent 高危操作确认回调（AGNT-020~022）
func NewConfirmAssistantToolLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfirmAssistantToolLogic {
	return &ConfirmAssistantToolLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ConfirmAssistantToolLogic) ConfirmAssistantTool(req *types.AssistantToolConfirmReq) (resp *types.AssistantToolConfirmResp, err error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	requestID := strings.TrimSpace(req.RequestId)
	callID := strings.TrimSpace(req.CallId)
	if requestID == "" || callID == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if _, err := l.svcCtx.AssistantService.ConfirmToolCall(l.ctx, &assistantservice.ConfirmToolCallReq{
		UserId:    userID,
		RequestId: requestID,
		CallId:    callID,
		Approved:  req.Approved,
	}); err != nil {
		l.Errorw("AssistantService.ConfirmToolCall RPC failed",
			logx.Field("userId", userID),
			logx.Field("requestId", requestID),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}
	return &types.AssistantToolConfirmResp{}, nil
}
