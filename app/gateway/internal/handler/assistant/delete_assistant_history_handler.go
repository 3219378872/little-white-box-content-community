package assistant

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/assistant"
	"esx/app/gateway/internal/svc"
	"esx/pkg/httpx"
)

// 清除 Assistant 历史
func DeleteAssistantHistoryHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		l := assistant.NewDeleteAssistantHistoryLogic(ctx, svcCtx)
		resp, err := l.DeleteAssistantHistory()
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
