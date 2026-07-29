package logic

import (
	"context"
	"esx/app/message/rpc/internal/model"
	"esx/app/message/rpc/internal/svc"
	"esx/app/message/rpc/xiaobaihe/message/pb"
	"strings"

	"errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SendMessageLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSendMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendMessageLogic {
	return &SendMessageLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

// 发送私信
func (l *SendMessageLogic) SendMessage(in *pb.SendMessageReq) (*pb.SendMessageResp, error) {
	if in == nil {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	content := strings.TrimSpace(in.Content)
	idempotencyKey := strings.TrimSpace(in.IdempotencyKey)
	if in.SenderId <= 0 ||
		in.ReceiverId <= 0 ||
		in.SenderId == in.ReceiverId ||
		content == "" ||
		!validMessageType(in.MsgType) ||
		runeLen(content) > maxMessageContentLength ||
		idempotencyKey == "" ||
		len(idempotencyKey) > maxMessageIdempotencyKeySize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	result, err := l.svcCtx.MessageCommandModel.CreateMessageWithConversations(l.ctx, in.SenderId, in.ReceiverId, content, int64(in.MsgType), idempotencyKey)
	if err != nil {
		if model.IsIdempotencyConflict(err) {
			return nil, errx.New(errx.ParamError, "幂等键已用于其他消息")
		}
		l.Errorw("MessageCommandModel.CreateMessageWithConversations failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if result.Created && l.svcCtx.UnreadStore != nil {
		if err := l.svcCtx.UnreadStore.DeleteUserUnread(l.ctx, in.ReceiverId); err != nil {
			l.Errorw("UnreadStore.DeleteUserUnread failed", logx.Field("err", err.Error()))
		}
	}
	return &pb.SendMessageResp{MessageId: result.MessageID}, nil
}
