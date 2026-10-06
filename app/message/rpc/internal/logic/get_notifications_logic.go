package logic

import (
	"context"
	"esx/app/message/rpc/internal/svc"
	pb "esx/kitex_gen/message"

	"esx/pkg/errx"

	"esx/pkg/logging"
)

type GetNotificationsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewGetNotificationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetNotificationsLogic {
	return &GetNotificationsLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// 获取通知列表
func (l *GetNotificationsLogic) GetNotifications(in *pb.GetNotificationsReq) (*pb.GetNotificationsResp, error) {
	if in.UserId <= 0 || in.Type < 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	page, pageSize := normalizePage(in.Page, in.PageSize)
	rows, total, err := l.svcCtx.NotificationModel.FindByUser(l.ctx, in.UserId, int64(in.Type), page, pageSize)
	if err != nil {
		l.Errorw("NotificationModel.FindByUser failed", logging.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	items := make([]*pb.NotificationInfo, 0, len(rows))
	for _, row := range rows {
		items = append(items, toNotificationInfo(row))
	}
	return &pb.GetNotificationsResp{Notifications: items, Total: total}, nil
}
