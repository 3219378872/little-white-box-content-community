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

// MarkConversationReadLogic 承载 MarkConversationRead 接口的业务逻辑；每个请求新建一个实例。
type MarkConversationReadLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 标记会话已读
func NewMarkConversationReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkConversationReadLogic {
	return &MarkConversationReadLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// MarkConversationRead 把当前用户在该会话中的消息标记为已读。
func (l *MarkConversationReadLogic) MarkConversationRead(req *types.MarkConversationReadReq) (resp *types.MarkConversationReadResp, err error) {
	if req == nil || req.ConversationId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	result, err := l.svcCtx.MessageService.MarkRead(l.ctx, &messageservice.MarkReadReq{
		UserId:         userID,
		ConversationId: req.ConversationId,
	})
	if err != nil {
		l.Errorw("MessageService.MarkRead RPC failed",
			logging.Field("conversationId", req.ConversationId),
			logging.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		l.Error("MessageService.MarkRead returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.MarkConversationReadResp{}, nil
}
