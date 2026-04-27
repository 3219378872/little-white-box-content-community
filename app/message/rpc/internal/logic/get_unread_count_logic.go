package logic

import (
	"context"
	"esx/app/message/rpc/internal/svc"
	"esx/app/message/rpc/xiaobaihe/message/pb"

	"errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUnreadCountLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUnreadCountLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUnreadCountLogic {
	return &GetUnreadCountLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

// 获取未读数
func (l *GetUnreadCountLogic) GetUnreadCount(in *pb.GetUnreadCountReq) (*pb.GetUnreadCountResp, error) {
	if in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	messageUnread, err := l.getMessageUnread(in.UserId)
	if err != nil {
		return nil, err
	}
	notificationUnread, err := l.getNotificationUnread(in.UserId)
	if err != nil {
		return nil, err
	}
	return &pb.GetUnreadCountResp{MessageUnread: int32(messageUnread), NotificationUnread: int32(notificationUnread)}, nil
}

func (l *GetUnreadCountLogic) getMessageUnread(userID int64) (int64, error) {
	if l.svcCtx.UnreadStore != nil {
		count, ok, err := l.svcCtx.UnreadStore.GetMessageUnread(l.ctx, userID)
		if err == nil && ok {
			return count, nil
		}
		if err != nil {
			l.Errorw("UnreadStore.GetMessageUnread failed", logx.Field("err", err.Error()))
		}
	}
	count, err := l.svcCtx.MessageModel.CountUnreadByUser(l.ctx, userID)
	if err != nil {
		return 0, errx.Wrap(err, errx.SystemError)
	}
	if l.svcCtx.UnreadStore != nil {
		if err := l.svcCtx.UnreadStore.SetMessageUnread(l.ctx, userID, count); err != nil {
			l.Errorw("UnreadStore.SetMessageUnread failed", logx.Field("err", err.Error()))
		}
	}
	return count, nil
}

func (l *GetUnreadCountLogic) getNotificationUnread(userID int64) (int64, error) {
	if l.svcCtx.UnreadStore != nil {
		count, ok, err := l.svcCtx.UnreadStore.GetNotificationUnread(l.ctx, userID)
		if err == nil && ok {
			return count, nil
		}
		if err != nil {
			l.Errorw("UnreadStore.GetNotificationUnread failed", logx.Field("err", err.Error()))
		}
	}
	count, err := l.svcCtx.NotificationModel.CountUnread(l.ctx, userID)
	if err != nil {
		return 0, errx.Wrap(err, errx.SystemError)
	}
	if l.svcCtx.UnreadStore != nil {
		if err := l.svcCtx.UnreadStore.SetNotificationUnread(l.ctx, userID, count); err != nil {
			l.Errorw("UnreadStore.SetNotificationUnread failed", logx.Field("err", err.Error()))
		}
	}
	return count, nil
}
