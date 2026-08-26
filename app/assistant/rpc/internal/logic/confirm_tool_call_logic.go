package logic

import (
	"context"
	"errors"
	"strings"

	"esx/app/assistant/rpc/internal/agent"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ConfirmToolCallLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewConfirmToolCallLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfirmToolCallLogic {
	return &ConfirmToolCallLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// ConfirmToolCall 处理高危操作的用户裁决（AGNT-020~022）：校验凭据归属后写入
// 裁决，唤醒等待中的 delete_post 执行。凭据一次性：未知、过期或不属于该用户的
// callID 一律 ParamError 拒绝。
func (l *ConfirmToolCallLogic) ConfirmToolCall(in *pb.ConfirmToolCallReq) (*pb.ConfirmToolCallResp, error) {
	if in == nil || in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	requestID := strings.TrimSpace(in.RequestId)
	callID := strings.TrimSpace(in.CallId)
	if requestID == "" || callID == "" || !validOpaqueID(requestID) || !validOpaqueID(callID) {
		return nil, errx.New(errx.ParamError, "invalid confirmation identity")
	}
	if l.svcCtx == nil || l.svcCtx.AgentConfirms == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	if err := l.svcCtx.AgentConfirms.Resolve(l.ctx, in.UserId, requestID, callID, in.Approved); err != nil {
		if errors.Is(err, agent.ErrConfirmExpired) {
			l.Infow("agent confirmation unknown or expired",
				logx.Field("user_id", in.UserId),
				logx.Field("request_id", requestID),
				logx.Field("call_id", callID))
			return nil, errx.New(errx.ParamError, "confirmation is unknown or expired")
		}
		l.Errorw("agent confirmation resolve failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.ServiceUnavailable)
	}
	l.Infow("agent confirmation resolved",
		logx.Field("user_id", in.UserId),
		logx.Field("request_id", requestID),
		logx.Field("call_id", callID),
		logx.Field("approved", in.Approved))
	return &pb.ConfirmToolCallResp{}, nil
}
