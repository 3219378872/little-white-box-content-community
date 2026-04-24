package logic

import (
	"context"

	"errx"
	"esx/app/message/internal/svc"
	"esx/app/message/xiaobaihe/message/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type MarkReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewMarkReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkReadLogic {
	return &MarkReadLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

// 标记已读
func (l *MarkReadLogic) MarkRead(in *pb.MarkReadReq) (*pb.MarkReadResp, error) {
	if in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if in.ConversationId > 0 {
		if _, err := l.svcCtx.MessageModel.MarkConversationRead(l.ctx, in.UserId, in.ConversationId); err != nil {
			l.Errorw("MessageModel.MarkConversationRead failed", logx.Field("err", err.Error()))
			return nil, errx.Wrap(err, errx.SystemError)
		}
	}
	if _, err := l.svcCtx.NotificationModel.MarkAllRead(l.ctx, in.UserId); err != nil {
		l.Errorw("NotificationModel.MarkAllRead failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if l.svcCtx.UnreadStore != nil {
		if err := l.svcCtx.UnreadStore.DeleteUserUnread(l.ctx, in.UserId); err != nil {
			l.Errorw("UnreadStore.DeleteUserUnread failed", logx.Field("err", err.Error()))
		}
	}
	return &pb.MarkReadResp{}, nil
}
