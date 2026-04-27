package logic

import (
	"context"
	"database/sql"
	"strings"

	"errx"
	"esx/app/message/internal/model"
	"esx/app/message/internal/svc"
	"esx/app/message/xiaobaihe/message/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type SendNotificationLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSendNotificationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendNotificationLogic {
	return &SendNotificationLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
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
		l.Errorw("NotificationModel.Insert failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if l.svcCtx.UnreadStore != nil {
		if err := l.svcCtx.UnreadStore.DeleteUserUnread(l.ctx, in.UserId); err != nil {
			l.Errorw("UnreadStore.DeleteUserUnread failed", logx.Field("err", err.Error()))
		}
	}
	return &pb.SendNotificationResp{NotificationId: id}, nil
}
