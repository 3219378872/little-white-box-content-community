package logic

import (
	"context"
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
	content := strings.TrimSpace(in.Content)
	if in.SenderId <= 0 ||
		in.ReceiverId <= 0 ||
		in.SenderId == in.ReceiverId ||
		content == "" ||
		!validMessageType(in.MsgType) ||
		runeLen(content) > maxMessageContentLength {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	id, err := l.svcCtx.MessageCommandModel.CreateMessageWithConversations(l.ctx, in.SenderId, in.ReceiverId, content, int64(in.MsgType))
	if err != nil {
		l.Errorw("MessageCommandModel.CreateMessageWithConversations failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if l.svcCtx.UnreadStore != nil {
		if err := l.svcCtx.UnreadStore.DeleteUserUnread(l.ctx, in.ReceiverId); err != nil {
			l.Errorw("UnreadStore.DeleteUserUnread failed", logx.Field("err", err.Error()))
		}
	}
	return &pb.SendMessageResp{MessageId: id}, nil
}
