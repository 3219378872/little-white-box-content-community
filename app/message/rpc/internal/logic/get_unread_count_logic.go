package logic

import (
	"context"
	"esx/app/message/rpc/internal/svc"
	"esx/app/message/rpc/xiaobaihe/message/pb"

	"esx/pkg/errx"

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
	count, err := l.svcCtx.MessageModel.CountUnreadByUser(l.ctx, userID)
	if err != nil {
		return 0, errx.Wrap(err, errx.SystemError)
	}
	return count, nil
}

func (l *GetUnreadCountLogic) getNotificationUnread(userID int64) (int64, error) {
	count, err := l.svcCtx.NotificationModel.CountUnread(l.ctx, userID)
	if err != nil {
		return 0, errx.Wrap(err, errx.SystemError)
	}
	return count, nil
}
