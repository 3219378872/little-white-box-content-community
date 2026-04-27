package logic

import (
	"context"

	"errx"
	"esx/app/message/internal/svc"
	"esx/app/message/xiaobaihe/message/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetConversationsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetConversationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetConversationsLogic {
	return &GetConversationsLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

// 获取会话列表
func (l *GetConversationsLogic) GetConversations(in *pb.GetConversationsReq) (*pb.GetConversationsResp, error) {
	if in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	page, pageSize := normalizePage(in.Page, in.PageSize)
	rows, total, err := l.svcCtx.ConversationModel.FindByUser(l.ctx, in.UserId, page, pageSize)
	if err != nil {
		l.Errorw("ConversationModel.FindByUser failed", logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	items := make([]*pb.ConversationInfo, 0, len(rows))
	for _, row := range rows {
		items = append(items, toConversationInfo(row))
	}
	return &pb.GetConversationsResp{Conversations: items, Total: total}, nil
}
