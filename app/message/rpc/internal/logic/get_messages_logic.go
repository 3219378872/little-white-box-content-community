package logic

import (
	"context"
	"errors"
	"esx/app/message/rpc/internal/model"
	"esx/app/message/rpc/internal/svc"
	"esx/app/message/rpc/xiaobaihe/message/pb"

	"errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMessagesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetMessagesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMessagesLogic {
	return &GetMessagesLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

// 获取聊天记录
func (l *GetMessagesLogic) GetMessages(in *pb.GetMessagesReq) (*pb.GetMessagesResp, error) {
	if in.UserId <= 0 || in.ConversationId <= 0 || in.LastId < 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	conversation, err := l.svcCtx.ConversationModel.FindOneForUser(l.ctx, in.UserId, in.ConversationId)
	if err != nil {
		l.Errorw("ConversationModel.FindOneForUser failed", logx.Field("err", err.Error()))
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.PermissionDenied)
		}
		return nil, errx.Wrap(err, errx.SystemError)
	}
	_, pageSize := normalizePage(1, in.PageSize)
	rows, hasMore, err := l.svcCtx.MessageModel.FindByUserConversation(l.ctx, in.UserId, conversation.TargetUserId, in.LastId, pageSize)
	if err != nil {
		l.Errorw("MessageModel.FindByUserConversation failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	items := make([]*pb.MessageInfo, 0, len(rows))
	for _, row := range rows {
		items = append(items, toMessageInfo(row))
	}
	return &pb.GetMessagesResp{Messages: items, HasMore: hasMore}, nil
}
