package logic

import (
	"context"
	"esx/app/message/rpc/internal/model"
	"esx/app/message/rpc/internal/svc"
	pb "esx/kitex_gen/message"
	"esx/pkg/errx"
	"strings"

	"esx/pkg/logging"
)

// SendMessageLogic 承载 SendMessage 接口的业务逻辑；每个请求新建一个实例。
type SendMessageLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewSendMessageLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewSendMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendMessageLogic {
	return &SendMessageLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
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
	// CORE-041：媒体消息必须引用已成功上传且属于发送者的媒体。
	if in.MsgType != messageTypeText {
		url, err := validateMessageMedia(l.ctx, l.Logger, l.svcCtx.MediaService, in.SenderId, in.MediaId, in.MsgType)
		if err != nil {
			return nil, err
		}
		content = url
	}
	result, err := l.svcCtx.MessageCommandModel.CreateMessageWithConversations(l.ctx, in.SenderId, in.ReceiverId, content, int64(in.MsgType), in.MediaId, idempotencyKey)
	if err != nil {
		if model.IsIdempotencyConflict(err) {
			return nil, errx.NewWithCode(errx.IdempotencyConflict)
		}
		l.Errorw("MessageCommandModel.CreateMessageWithConversations failed", logging.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if result.Created && l.svcCtx.UnreadStore != nil {
		if err := l.svcCtx.UnreadStore.DeleteUserUnread(l.ctx, in.ReceiverId); err != nil {
			l.Errorw("UnreadStore.DeleteUserUnread failed", logging.Field("err", err.Error()))
		}
	}
	return &pb.SendMessageResp{MessageId: result.MessageID}, nil
}
