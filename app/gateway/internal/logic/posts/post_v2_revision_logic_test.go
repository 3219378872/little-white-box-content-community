// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/rest/httpx"
	"google.golang.org/grpc"
)

// CORE-013：v2 写接口强制乐观锁，缺失或为 0 的 expectedRevision 必须被拒绝。
func TestUpdatePostV2RejectsMissingOrZeroRevision(t *testing.T) {
	l := NewUpdatePostV2Logic(context.Background(), &svc.ServiceContext{})
	for name, req := range map[string]*types.UpdatePostV2Req{
		"missing": {PostId: 11},
		"zero":    {PostId: 11, ExpectedRevision: 0},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := l.UpdatePostV2(req)
			assert.True(t, errx.Is(err, errx.ParamError), "want ParamError, got %v", err)
		})
	}
}

type capturePostUpdate struct {
	contentservice.ContentService
	request *contentservice.UpdatePostReq
}

func (c *capturePostUpdate) UpdatePost(_ context.Context, request *contentservice.UpdatePostReq, _ ...grpc.CallOption) (*contentservice.UpdatePostResp, error) {
	c.request = request
	return &contentservice.UpdatePostResp{Status: 1, Revision: 3}, nil
}

func TestUpdatePostV2PreservesOptionalImagePresence(t *testing.T) {
	for _, test := range []struct {
		name, body string
		present    bool
	}{
		{"omitted", `{"title":"title","expectedRevision":2}`, false},
		{"empty", `{"images":[],"mediaIds":[],"expectedRevision":2}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("PUT", "/api/v2/post/1", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			var input types.UpdatePostV2Req
			require.NoError(t, httpx.ParseJsonBody(request, &input))
			input.PostId = 1
			content := new(capturePostUpdate)
			ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 1})
			_, err := NewUpdatePostV2Logic(ctx, &svc.ServiceContext{ContentService: content}).UpdatePostV2(&input)
			require.NoError(t, err)
			assert.Equal(t, test.present, content.request.ImagesProvided)
			assert.Equal(t, test.present, content.request.MediaIdsProvided)
		})
	}
}

func TestDeletePostV2RejectsMissingOrZeroRevision(t *testing.T) {
	l := NewDeletePostV2Logic(context.Background(), &svc.ServiceContext{})
	for name, req := range map[string]*types.DeletePostV2Req{
		"missing": {PostId: 11},
		"zero":    {PostId: 11, ExpectedRevision: 0},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := l.DeletePostV2(req)
			assert.True(t, errx.Is(err, errx.ParamError), "want ParamError, got %v", err)
		})
	}
}
