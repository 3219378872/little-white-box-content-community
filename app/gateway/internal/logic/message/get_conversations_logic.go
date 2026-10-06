package message

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/message/rpc/messageservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

type GetConversationsLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取会话列表
func NewGetConversationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetConversationsLogic {
	return &GetConversationsLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetConversationsLogic) GetConversations(req *types.GetConversationsReq) (resp *types.GetConversationsResp, err error) {
	if req == nil || req.Page <= 0 || req.PageSize <= 0 || req.PageSize > maxMessagePageSize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	result, err := l.svcCtx.MessageService.GetConversations(l.ctx, &messageservice.GetConversationsReq{
		UserId:   userID,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		l.Errorw("MessageService.GetConversations RPC failed", logging.Field("err", err.Error()))
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		l.Error("MessageService.GetConversations returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.GetConversationsResp{
		Conversations: conversationItems(result.Conversations),
		Total:         result.Total,
	}, nil
}
