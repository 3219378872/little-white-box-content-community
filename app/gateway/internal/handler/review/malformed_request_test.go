package review

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/stretchr/testify/require"

	"esx/app/gateway/internal/svc"
	"esx/pkg/httptestx"
	"esx/pkg/httpx"
)

// 请求体无法解析时，handler 必须在调用 logic 之前以参数错误返回；
// svcCtx 不带任何下游客户端，误入 logic 会直接 panic。
func TestHandlersRejectMalformedJSONBeforeLogic(t *testing.T) {
	wantStatus, wantBody := httpx.MapError(httpx.ParamError())
	wantJSON, err := json.Marshal(wantBody)
	require.NoError(t, err)
	handlers := map[string]func(*svc.ServiceContext) app.HandlerFunc{
		"ClaimReviewTaskHandler":        ClaimReviewTaskHandler,
		"ConfirmReviewSeedHandler":      ConfirmReviewSeedHandler,
		"GetReviewEvidenceMediaHandler": GetReviewEvidenceMediaHandler,
		"GetReviewQueueHandler":         GetReviewQueueHandler,
		"GetReviewTaskHandler":          GetReviewTaskHandler,
		"GetReviewerProfileHandler":     GetReviewerProfileHandler,
		"ListReviewSeedsHandler":        ListReviewSeedsHandler,
		"ReleaseReviewTaskHandler":      ReleaseReviewTaskHandler,
		"RenewReviewTaskHandler":        RenewReviewTaskHandler,
		"RetireReviewSeedHandler":       RetireReviewSeedHandler,
		"SubmitReviewDecisionHandler":   SubmitReviewDecisionHandler,
	}
	for name, newHandler := range handlers {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			httptestx.Adapt(newHandler(&svc.ServiceContext{}))(rec, req)

			require.Equal(t, wantStatus, rec.Code)
			require.JSONEq(t, string(wantJSON), rec.Body.String())
		})
	}
}
