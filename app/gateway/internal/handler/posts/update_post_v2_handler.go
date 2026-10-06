package posts

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/posts"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/httpx"
)

// 更新帖子（v2，强制 expectedRevision）
func UpdatePostV2Handler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req types.UpdatePostV2Req
		if err := httpx.Parse(c, &req); err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}

		l := posts.NewUpdatePostV2Logic(ctx, svcCtx)
		resp, err := l.UpdatePostV2(&req)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
