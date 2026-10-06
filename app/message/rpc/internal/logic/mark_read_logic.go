package logic

import (
	"context"
	"errors"
	"esx/app/message/rpc/internal/model"
	"esx/app/message/rpc/internal/svc"
	pb "esx/kitex_gen/message"

	"esx/pkg/errx"

	"esx/pkg/logging"
)

// MarkReadLogic 承载 MarkRead 接口的业务逻辑；每个请求新建一个实例。
type MarkReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewMarkReadLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewMarkReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkReadLogic {
	return &MarkReadLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// 标记已读
func (l *MarkReadLogic) MarkRead(in *pb.MarkReadReq) (*pb.MarkReadResp, error) {
	if in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if in.ConversationId > 0 {
		conversation, err := l.svcCtx.ConversationModel.FindOneForUser(l.ctx, in.UserId, in.ConversationId)
		if err != nil {
			l.Errorw("ConversationModel.FindOneForUser failed", logging.Field("err", err.Error()))
			if errors.Is(err, model.ErrNotFound) {
				return nil, errx.NewWithCode(errx.PermissionDenied)
			}
			return nil, errx.Wrap(err, errx.SystemError)
		}
		if _, err := l.svcCtx.MessageCommandModel.MarkConversationRead(l.ctx, in.UserId, conversation.TargetUserId); err != nil {
			l.Errorw("MessageCommandModel.MarkConversationRead failed", logging.Field("err", err.Error()))
			return nil, errx.Wrap(err, errx.SystemError)
		}
	} else {
		if _, err := l.svcCtx.NotificationModel.MarkAllRead(l.ctx, in.UserId); err != nil {
			l.Errorw("NotificationModel.MarkAllRead failed", logging.Field("err", err.Error()))
			return nil, errx.Wrap(err, errx.SystemError)
		}
	}
	return &pb.MarkReadResp{}, nil
}
