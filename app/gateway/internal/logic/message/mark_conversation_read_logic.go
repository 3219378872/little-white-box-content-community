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

type MarkConversationReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 标记会话已读
func NewMarkConversationReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkConversationReadLogic {
	return &MarkConversationReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *MarkConversationReadLogic) MarkConversationRead(req *types.MarkConversationReadReq) (resp *types.MarkConversationReadResp, err error) {
	if req == nil || req.ConversationId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil || userID <= 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}

	result, err := l.svcCtx.MessageService.MarkRead(l.ctx, &messageservice.MarkReadReq{
		UserId:         userID,
		ConversationId: req.ConversationId,
	})
	if err != nil {
		l.Errorw("MessageService.MarkRead RPC failed",
			logx.Field("conversationId", req.ConversationId),
			logx.Field("err", err.Error()),
		)
		return nil, messageRPCError(err)
	}
	if result == nil {
		l.Error("MessageService.MarkRead returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.MarkConversationReadResp{}, nil
}
