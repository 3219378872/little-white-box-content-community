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

// GetMessagesLogic 承载 GetMessages 接口的业务逻辑；每个请求新建一个实例。
type GetMessagesLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取会话消息
func NewGetMessagesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMessagesLogic {
	return &GetMessagesLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetMessages 以 LastId 为游标向前翻页读取会话消息；会话归属由消息服务校验。
func (l *GetMessagesLogic) GetMessages(req *types.GetMessagesReq) (resp *types.GetMessagesResp, err error) {
	if req == nil || req.ConversationId <= 0 || req.LastId < 0 || req.PageSize <= 0 || req.PageSize > maxMessagePageSize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	result, err := l.svcCtx.MessageService.GetMessages(l.ctx, &messageservice.GetMessagesReq{
		ConversationId: req.ConversationId,
		LastId:         req.LastId,
		PageSize:       req.PageSize,
		UserId:         userID,
	})
	if err != nil {
		l.Errorw("MessageService.GetMessages RPC failed",
			logging.Field("conversationId", req.ConversationId),
			logging.Field("err", err.Error()),
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
