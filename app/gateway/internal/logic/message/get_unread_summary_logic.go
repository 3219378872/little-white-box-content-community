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

type GetUnreadSummaryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取未读汇总
func NewGetUnreadSummaryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUnreadSummaryLogic {
	return &GetUnreadSummaryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUnreadSummaryLogic) GetUnreadSummary() (resp *types.GetUnreadSummaryResp, err error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil || userID <= 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}

	result, err := l.svcCtx.MessageService.GetUnreadCount(l.ctx, &messageservice.GetUnreadCountReq{UserId: userID})
	if err != nil {
		l.Errorw("MessageService.GetUnreadCount RPC failed", logx.Field("err", err.Error()))
		return nil, errx.FromRPCError(err)
	}
	if result == nil {
		l.Error("MessageService.GetUnreadCount returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.GetUnreadSummaryResp{
		MessageUnread:      result.MessageUnread,
		NotificationUnread: result.NotificationUnread,
	}, nil
}
