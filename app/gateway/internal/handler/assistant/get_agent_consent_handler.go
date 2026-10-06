package assistant

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/assistant"
	"esx/app/gateway/internal/svc"
	"esx/pkg/httpx"
)

// 查询 Agent 能力授权状态（AGNT-004）
func GetAgentConsentHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		l := assistant.NewGetAgentConsentLogic(ctx, svcCtx)
		resp, err := l.GetAgentConsent()
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
