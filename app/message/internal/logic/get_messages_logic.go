package logic

import (
	"context"

	"errx"
	"esx/app/message/internal/svc"
	"esx/app/message/xiaobaihe/message/pb"

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
	if in.ConversationId <= 0 || in.LastId < 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	_, pageSize := normalizePage(1, in.PageSize)
	rows, hasMore, err := l.svcCtx.MessageModel.FindByConversation(l.ctx, in.ConversationId, in.LastId, pageSize)
	if err != nil {
		l.Errorw("MessageModel.FindByConversation failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	items := make([]*pb.MessageInfo, 0, len(rows))
	for _, row := range rows {
		items = append(items, toMessageInfo(row))
	}
	return &pb.GetMessagesResp{Messages: items, HasMore: hasMore}, nil
}
