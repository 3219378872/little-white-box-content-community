package review

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"

	"esx/app/gateway/internal/logic/review"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/httpx"
)

// 审核工作台：POST /api/v2/review/tasks/claim
func ClaimReviewTaskHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req types.ClaimReviewTaskReq
		if err := httpx.Parse(c, &req); err != nil {
			httpx.ErrorCtx(ctx, c, err)
			return
		}

		l := review.NewClaimReviewTaskLogic(ctx, svcCtx)
		resp, err := l.ClaimReviewTask(&req)
		if err != nil {
			httpx.ErrorCtx(ctx, c, err)
		} else {
			httpx.OkJsonCtx(ctx, c, resp)
		}
	}
}
