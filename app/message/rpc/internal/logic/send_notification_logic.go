package logic

import (
	"context"
	"database/sql"
	"esx/app/message/rpc/internal/model"
	"esx/app/message/rpc/internal/svc"
	pb "esx/kitex_gen/message"
	"strings"

	"esx/pkg/errx"

	"esx/pkg/logging"
)

// SendNotificationLogic 承载 SendNotification 接口的业务逻辑；每个请求新建一个实例。
type SendNotificationLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewSendNotificationLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewSendNotificationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendNotificationLogic {
	return &SendNotificationLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// 发送系统通知
func (l *SendNotificationLogic) SendNotification(in *pb.SendNotificationReq) (*pb.SendNotificationResp, error) {
	title := strings.TrimSpace(in.Title)
	content := strings.TrimSpace(in.Content)
	if in.UserId <= 0 ||
		!validNotificationType(in.Type) ||
		content == "" ||
		runeLen(title) > maxNotificationTitleLength ||
		runeLen(content) > maxNotificationContentLength {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	row := &model.Notification{
		UserId:   in.UserId,
		Type:     int64(in.Type),
		Title:    nullableString(title),
		Content:  nullableString(content),
		TargetId: sql.NullInt64{Int64: in.TargetId, Valid: in.TargetId > 0},
		Status:   0,
	}
	result, err := l.svcCtx.NotificationModel.Insert(l.ctx, row)
	if err != nil {
		l.Errorw("NotificationModel.Insert failed", logging.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, errx.Wrap(err, errx.SystemError)
	}
	return &pb.SendNotificationResp{NotificationId: id}, nil
}
