package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestSafeAccessLogPreservesStatusAndBody(t *testing.T) {
	rec := performHertz(NewSafeAccessLogMiddleware().Hertz, func(_ context.Context, c *app.RequestContext) {
		c.String(http.StatusTeapot, "response")
	}, http.MethodPost, "/private?token=hidden")

	resp := rec.Result()
	if resp.StatusCode() != http.StatusTeapot || string(resp.Body()) != "response" {
		t.Fatalf("status=%d body=%q", resp.StatusCode(), string(resp.Body()))
	}
}
