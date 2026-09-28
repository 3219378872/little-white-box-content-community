package assistant

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/assistant"
	"esx/app/gateway/internal/svc"
	"esx/pkg/httpx"
)

// 标记 Assistant 线程已读
func MarkAssistantThreadReadHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		l := assistant.NewMarkAssistantThreadReadLogic(ctx, svcCtx)
		resp, err := l.MarkAssistantThreadRead()
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
