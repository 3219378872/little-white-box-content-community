// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package message

import (
	"context"

	"errx"
	"esx/app/message/rpc/messageservice"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMessagesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取会话消息
func NewGetMessagesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMessagesLogic {
	return &GetMessagesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetMessagesLogic) GetMessages(req *types.GetMessagesReq) (resp *types.GetMessagesResp, err error) {
	if req == nil || req.ConversationId <= 0 || req.LastId < 0 || req.PageSize <= 0 || req.PageSize > maxMessagePageSize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil || userID <= 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}

	result, err := l.svcCtx.MessageService.GetMessages(l.ctx, &messageservice.GetMessagesReq{
		ConversationId: req.ConversationId,
		LastId:         req.LastId,
		PageSize:       req.PageSize,
		UserId:         userID,
	})
	if err != nil {
		l.Errorw("MessageService.GetMessages RPC failed",
			logx.Field("conversationId", req.ConversationId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		l.Error("MessageService.GetMessages returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.GetMessagesResp{
		Messages: messageItems(result.Messages),
		HasMore:  result.HasMore,
	}, nil
}
