package message

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/message"
	"esx/app/gateway/internal/svc"
	"esx/pkg/httpx"
)

// 获取未读汇总
func GetUnreadSummaryHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		l := message.NewGetUnreadSummaryLogic(ctx, svcCtx)
		resp, err := l.GetUnreadSummary()
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
