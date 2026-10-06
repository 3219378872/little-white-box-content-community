package message

import (
	"context"
	"strings"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/message/rpc/messageservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// SendMessageLogic 承载 SendMessage 接口的业务逻辑；每个请求新建一个实例。
type SendMessageLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 发送私信
func NewSendMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendMessageLogic {
	return &SendMessageLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// SendMessage 校验消息类型、内容与幂等键后发送私信；不允许给自己发消息。
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
	if err != nil {
		return nil, err
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
		l.Errorw("MessageService.SendMessage RPC failed", logging.Field("err", err.Error()))
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		l.Error("MessageService.SendMessage returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.SendMessageResp{MessageId: result.MessageId}, nil
}
