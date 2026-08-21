// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package message

import (
	"context"
	"strings"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/message/rpc/messageservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SendMessageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 发送私信
func NewSendMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendMessageLogic {
	return &SendMessageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SendMessageLogic) SendMessage(req *types.SendMessageReq) (resp *types.SendMessageResp, err error) {
	if req == nil || req.ReceiverId <= 0 || req.MsgType < 1 || req.MsgType > 4 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	content := strings.TrimSpace(req.Content)
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if content == "" || idempotencyKey == "" || len(idempotencyKey) > 128 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil || userID <= 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if userID == req.ReceiverId {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	result, err := l.svcCtx.MessageService.SendMessage(l.ctx, &messageservice.SendMessageReq{
		SenderId:       userID,
		ReceiverId:     req.ReceiverId,
		Content:        content,
		MsgType:        req.MsgType,
		IdempotencyKey: idempotencyKey,
		MediaId:        req.MediaId,
	})
	if err != nil {
		l.Errorw("MessageService.SendMessage RPC failed", logx.Field("err", err.Error()))
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		l.Error("MessageService.SendMessage returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.SendMessageResp{MessageId: result.MessageId}, nil
}
