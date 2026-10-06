package logic

import (
	"context"
	"esx/app/message/rpc/internal/svc"
	pb "esx/kitex_gen/message"

	"esx/pkg/errx"

	"esx/pkg/logging"
)

// GetUnreadCountLogic 承载 GetUnreadCount 接口的业务逻辑；每个请求新建一个实例。
type GetUnreadCountLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewGetUnreadCountLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetUnreadCountLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUnreadCountLogic {
	return &GetUnreadCountLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
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

// getMessageUnread 从数据库统计未读私信数。
func (l *GetUnreadCountLogic) getMessageUnread(userID int64) (int64, error) {
	count, err := l.svcCtx.MessageModel.CountUnreadByUser(l.ctx, userID)
	if err != nil {
		return 0, errx.Wrap(err, errx.SystemError)
	}
	return count, nil
}

// getNotificationUnread 从数据库统计未读通知数。
func (l *GetUnreadCountLogic) getNotificationUnread(userID int64) (int64, error) {
	count, err := l.svcCtx.NotificationModel.CountUnread(l.ctx, userID)
	if err != nil {
		return 0, errx.Wrap(err, errx.SystemError)
	}
	return count, nil
}
