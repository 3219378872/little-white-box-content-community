package logic

import (
	"context"
	"strings"

	"errx"
	"esx/app/message/internal/model"
	"esx/app/message/internal/svc"
	"esx/app/message/xiaobaihe/message/pb"

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
	if in.SenderId <= 0 || in.ReceiverId <= 0 || in.SenderId == in.ReceiverId || content == "" || in.MsgType <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	_, receiverConversationID, err := l.svcCtx.ConversationModel.UpsertPairForMessage(l.ctx, in.SenderId, in.ReceiverId, content)
	if err != nil {
		l.Errorw("ConversationModel.UpsertPairForMessage failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	result, err := l.svcCtx.MessageModel.Insert(l.ctx, &model.Message{
		ConversationId: receiverConversationID,
		SenderId:       in.SenderId,
		ReceiverId:     in.ReceiverId,
		Content:        content,
		MsgType:        int64(in.MsgType),
		Status:         0,
	})
	if err != nil {
		l.Errorw("MessageModel.Insert failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if l.svcCtx.UnreadStore != nil {
		if err := l.svcCtx.UnreadStore.DeleteUserUnread(l.ctx, in.ReceiverId); err != nil {
			l.Errorw("UnreadStore.DeleteUserUnread failed", logx.Field("err", err.Error()))
		}
	}
	return &pb.SendMessageResp{MessageId: id}, nil
}
