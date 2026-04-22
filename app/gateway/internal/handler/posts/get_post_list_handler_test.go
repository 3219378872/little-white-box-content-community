package posts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"esx/app/content/contentservice"
	contentpb "esx/app/content/pb/xiaobaihe/content/pb"
	"gateway/internal/config"
	"gateway/internal/svc"
	"google.golang.org/grpc"
	"jwtx"
)

type fakeContentService struct {
	contentservice.ContentService
	getPostListFn func(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error)
}

func (f *fakeContentService) GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	return f.getPostListFn(ctx, in, opts...)
}

func newPostListSvcCtx() *svc.ServiceContext {
	return &svc.ServiceContext{
		Config: config.Config{
			Auth: struct {
				AccessSecret string
				AccessExpire int64
			}{
				AccessSecret: "secret",
				AccessExpire: 3600,
			},
		},
		ContentService: &fakeContentService{
			getPostListFn: func(_ context.Context, _ *contentservice.GetPostListReq, _ ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
				return &contentservice.GetPostListResp{
					Posts: []*contentpb.PostInfo{
						{
							Id:        1,
							AuthorId:  2,
							Title:     "t",
							Content:   "c",
							Images:    []string{"a"},
							Tags:      []string{"tag"},
							ViewCount: 3,
							LikeCount: 4,
							CreatedAt: 5,
						},
					},
					Total: 1,
				}, nil
			},
		},
	}
}

func TestGetPostListHandler_NoToken_Returns200AndAnonymous(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?page=1&pageSize=20&sortBy=1", nil)
	rec := httptest.NewRecorder()

	GetPostListHandler(newPostListSvcCtx())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Auth-State"); got != "anonymous" {
		t.Fatalf("expected anonymous auth state, got %q", got)
	}

	var resp struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v, body=%s", err, rec.Body.String())
	}
	if resp.Total != 1 || len(resp.List) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGetPostListHandler_ExpiredToken_Returns200AndExpired(t *testing.T) {
	token, err := jwtx.GenerateToken(42, "alice", jwtx.JwtConfig{
		AccessSecret: "secret",
		AccessExpire: -1,
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?page=1&pageSize=20&sortBy=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	GetPostListHandler(newPostListSvcCtx())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Auth-State"); got != "expired" {
		t.Fatalf("expected expired auth state, got %q", got)
	}
}

func TestGetPostListHandler_ValidToken_Returns200AndAuthenticated(t *testing.T) {
	token, err := jwtx.GenerateToken(42, "alice", jwtx.JwtConfig{
		AccessSecret: "secret",
		AccessExpire: 3600,
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?page=1&pageSize=20&sortBy=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	GetPostListHandler(newPostListSvcCtx())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Auth-State"); got != "authenticated" {
		t.Fatalf("expected authenticated auth state, got %q", got)
	}
}
