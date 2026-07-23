package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"errx"
	"esx/app/content/rpc/contentservice"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/media/rpc/mediaservice"
	mediapb "esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"gateway/internal/config"
	"gateway/internal/handler"
	"gateway/internal/httpxconfig"
	"gateway/internal/svc"
	"jwtx"
	"middleware"
	userpb "user/pb/xiaobaihe/user/pb"
	"user/userservice"

	"github.com/zeromicro/go-zero/rest"
	"google.golang.org/grpc"
)

const contractSecret = "rest-decision-table-secret"

type contractUserService struct{ userservice.UserService }

func (contractUserService) GetUser(context.Context, *userservice.GetUserReq, ...grpc.CallOption) (*userservice.GetUserResp, error) {
	return &userservice.GetUserResp{User: &userpb.UserInfo{Id: 2, Username: "user", FavoritesVisibility: 1}}, nil
}
func (contractUserService) UpdateProfile(context.Context, *userservice.UpdateProfileReq, ...grpc.CallOption) (*userservice.UpdateProfileResp, error) {
	return &userservice.UpdateProfileResp{}, nil
}
func (contractUserService) Follow(context.Context, *userservice.FollowReq, ...grpc.CallOption) (*userservice.FollowResp, error) {
	return &userservice.FollowResp{}, nil
}
func (contractUserService) Unfollow(context.Context, *userservice.UnfollowReq, ...grpc.CallOption) (*userservice.UnfollowResp, error) {
	return &userservice.UnfollowResp{}, nil
}
func (contractUserService) Register(context.Context, *userservice.RegisterReq, ...grpc.CallOption) (*userservice.RegisterResp, error) {
	return &userservice.RegisterResp{UserId: 1, Token: "token"}, nil
}
func (contractUserService) Login(context.Context, *userservice.LoginReq, ...grpc.CallOption) (*userservice.LoginResp, error) {
	return &userservice.LoginResp{UserId: 1, Token: "token"}, nil
}
func (contractUserService) SendVerifyCode(context.Context, *userservice.SendVerifyCodeReq, ...grpc.CallOption) (*userservice.SendVerifyCodeResp, error) {
	return &userservice.SendVerifyCodeResp{}, nil
}

type contractContentService struct{ contentservice.ContentService }

func contractPost() *contentpb.PostInfo {
	return &contentpb.PostInfo{Id: 11, AuthorId: 1, Title: "title", Content: "content", Status: 1}
}
func (contractContentService) CreatePost(context.Context, *contentservice.CreatePostReq, ...grpc.CallOption) (*contentservice.CreatePostResp, error) {
	return &contentservice.CreatePostResp{PostId: 11}, nil
}
func (contractContentService) GetPost(context.Context, *contentservice.GetPostReq, ...grpc.CallOption) (*contentservice.GetPostResp, error) {
	return &contentservice.GetPostResp{Post: contractPost()}, nil
}
func (contractContentService) UpdatePost(context.Context, *contentservice.UpdatePostReq, ...grpc.CallOption) (*contentservice.UpdatePostResp, error) {
	return &contentservice.UpdatePostResp{}, nil
}
func (contractContentService) DeletePost(context.Context, *contentservice.DeletePostReq, ...grpc.CallOption) (*contentservice.DeletePostResp, error) {
	return &contentservice.DeletePostResp{}, nil
}
func (contractContentService) GetPostList(context.Context, *contentservice.GetPostListReq, ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	return &contentservice.GetPostListResp{Posts: []*contentpb.PostInfo{contractPost()}, Total: 1}, nil
}
func (contractContentService) GetUserPosts(context.Context, *contentservice.GetUserPostsReq, ...grpc.CallOption) (*contentservice.GetUserPostsResp, error) {
	return &contentservice.GetUserPostsResp{Posts: []*contentpb.PostInfo{contractPost()}, Total: 1}, nil
}
func (contractContentService) GetPostsByIds(context.Context, *contentservice.GetPostsByIdsReq, ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error) {
	return &contentservice.GetPostsByIdsResp{Posts: []*contentpb.PostInfo{contractPost()}}, nil
}
func (contractContentService) CreateComment(context.Context, *contentservice.CreateCommentReq, ...grpc.CallOption) (*contentservice.CreateCommentResp, error) {
	return &contentservice.CreateCommentResp{CommentId: 21}, nil
}
func (contractContentService) DeleteComment(context.Context, *contentservice.DeleteCommentReq, ...grpc.CallOption) (*contentservice.DeleteCommentResp, error) {
	return &contentservice.DeleteCommentResp{}, nil
}
func (contractContentService) GetCommentList(context.Context, *contentservice.GetCommentListReq, ...grpc.CallOption) (*contentservice.GetCommentListResp, error) {
	return &contentservice.GetCommentListResp{Comments: []*contentpb.CommentInfo{{Id: 21, PostId: 11, UserId: 1, Content: "comment"}}, Total: 1}, nil
}

type contractInteractionService struct {
	interactionservice.InteractionService
}

func (contractInteractionService) Like(context.Context, *interactionservice.LikeReq, ...grpc.CallOption) (*interactionservice.LikeResp, error) {
	return &interactionservice.LikeResp{}, nil
}
func (contractInteractionService) Unlike(context.Context, *interactionservice.UnlikeReq, ...grpc.CallOption) (*interactionservice.UnlikeResp, error) {
	return &interactionservice.UnlikeResp{}, nil
}
func (contractInteractionService) Favorite(context.Context, *interactionservice.FavoriteReq, ...grpc.CallOption) (*interactionservice.FavoriteResp, error) {
	return &interactionservice.FavoriteResp{}, nil
}
func (contractInteractionService) Unfavorite(context.Context, *interactionservice.UnfavoriteReq, ...grpc.CallOption) (*interactionservice.UnfavoriteResp, error) {
	return &interactionservice.UnfavoriteResp{}, nil
}
func (contractInteractionService) GetFavoriteList(context.Context, *interactionservice.GetFavoriteListReq, ...grpc.CallOption) (*interactionservice.GetFavoriteListResp, error) {
	return &interactionservice.GetFavoriteListResp{PostIds: []int64{11}, Total: 1}, nil
}

type contractUploadStream struct{ grpc.ClientStream }

func (*contractUploadStream) Send(*mediapb.UploadImageReq) error { return nil }
func (*contractUploadStream) CloseAndRecv() (*mediapb.UploadImageResp, error) {
	return &mediapb.UploadImageResp{Media: &mediapb.MediaInfo{Id: 31, Url: "https://media/image.png"}}, nil
}

type contractMediaService struct{ mediaservice.MediaService }

func (contractMediaService) UploadImage(context.Context, ...grpc.CallOption) (mediapb.MediaService_UploadImageClient, error) {
	return &contractUploadStream{}, nil
}

type restDecision struct {
	id          string
	method      string
	path        string
	routePath   string
	body        func(t *testing.T) (io.Reader, string)
	auth        bool
	headerToken string
	wantStatus  int
	wantCode    int
	wantFields  []string
	wantHeaders map[string]string
}

func jsonBody(value string) func(*testing.T) (io.Reader, string) {
	return func(*testing.T) (io.Reader, string) {
		return bytes.NewBufferString(value), "application/json"
	}
}

func imageBody(t *testing.T) (io.Reader, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(map[string][]string)
	header["Content-Disposition"] = []string{`form-data; name="file"; filename="image.png"`}
	header["Content-Type"] = []string{"image/png"}
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte("png")); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

func plainBody(value string) func(*testing.T) (io.Reader, string) {
	return func(*testing.T) (io.Reader, string) {
		return bytes.NewBufferString(value), "text/plain"
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func startContractServer(t *testing.T) (string, []rest.Route) {
	t.Helper()
	port := freePort(t)
	cfg := config.Config{}
	cfg.RestConf = rest.RestConf{Host: "127.0.0.1", Port: port, Timeout: 3000}
	cfg.Auth.AccessSecret = contractSecret
	cfg.Auth.AccessExpire = 3600
	optionalAuth := middleware.NewOptionalAuthMiddleware(jwtx.JwtConfig{AccessSecret: contractSecret, AccessExpire: 3600})
	ctx := &svc.ServiceContext{
		Config:             cfg,
		UserService:        contractUserService{},
		ContentService:     contractContentService{},
		InteractionService: contractInteractionService{},
		MediaService:       contractMediaService{},
		OptionalAuth:       optionalAuth.Handle,
	}

	httpxconfig.ConfigureErrors()
	server := rest.MustNewServer(cfg.RestConf, rest.WithUnauthorizedCallback(httpxconfig.Unauthorized))
	handler.RegisterHandlers(server, ctx)
	routes := server.Routes()
	go server.Start()
	t.Cleanup(server.Stop)

	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/v1/health")
		if err == nil {
			_ = resp.Body.Close()
			return baseURL, routes
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("contract server did not become ready")
	return "", nil
}

func TestRESTDecisionTable(t *testing.T) {
	baseURL, routes := startContractServer(t)
	token, err := jwtx.GenerateToken(1, "alice", jwtx.JwtConfig{AccessSecret: contractSecret, AccessExpire: 3600})
	if err != nil {
		t.Fatal(err)
	}
	expiredToken, err := jwtx.GenerateToken(1, "alice", jwtx.JwtConfig{AccessSecret: contractSecret, AccessExpire: -1})
	if err != nil {
		t.Fatal(err)
	}

	successes := []restDecision{
		{id: "HEALTH-VALID", method: http.MethodGet, path: "/api/v1/health", wantStatus: http.StatusOK, wantFields: []string{"status"}},
		{id: "POST-LIST-ANON", method: http.MethodGet, path: "/api/v1/posts?page=1&pageSize=20&sortBy=1", wantStatus: http.StatusOK, wantFields: []string{"list", "total", "page", "pageSize"}, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateAnonymous}},
		{id: "AUTH-REGISTER-VALID", method: http.MethodPost, path: "/api/v1/auth/register", body: jsonBody(`{"username":"newuser","password":"Strong123"}`), wantStatus: http.StatusOK, wantFields: []string{"userId", "token"}},
		{id: "AUTH-LOGIN-VALID", method: http.MethodPost, path: "/api/v1/auth/login", body: jsonBody(`{"username":"alice","password":"Strong123","loginType":1}`), wantStatus: http.StatusOK, wantFields: []string{"userId", "token"}},
		{id: "AUTH-CODE-VALID", method: http.MethodPost, path: "/api/v1/auth/verify-code", body: jsonBody(`{"phone":"13800138000","type":1}`), wantStatus: http.StatusOK},
		{id: "USER-GET-VALID", method: http.MethodGet, path: "/api/v1/user/2", routePath: "/api/v1/user/:userId", wantStatus: http.StatusOK, wantFields: []string{"id", "username", "favoritesVisible"}},
		{id: "USER-POSTS-VALID", method: http.MethodGet, path: "/api/v1/users/2/posts?page=1&pageSize=20&sortBy=1", routePath: "/api/v1/users/:userId/posts", wantStatus: http.StatusOK, wantFields: []string{"list", "total", "page", "pageSize"}},
		{id: "USER-FAVORITES-VALID", method: http.MethodGet, path: "/api/v1/users/2/favorites?page=1&pageSize=20", routePath: "/api/v1/users/:userId/favorites", wantStatus: http.StatusOK, wantFields: []string{"list", "total", "page", "pageSize"}},
		{id: "USER-PROFILE-VALID", method: http.MethodPut, path: "/api/v1/user/profile", body: jsonBody(`{"nickname":"Alice"}`), auth: true, wantStatus: http.StatusOK},
		{id: "USER-FOLLOW-VALID", method: http.MethodPost, path: "/api/v1/user/follow", body: jsonBody(`{"targetUserId":2}`), auth: true, wantStatus: http.StatusOK},
		{id: "USER-UNFOLLOW-VALID", method: http.MethodDelete, path: "/api/v1/user/follow", body: jsonBody(`{"targetUserId":2}`), auth: true, wantStatus: http.StatusOK},
		{id: "POST-CREATE-VALID", method: http.MethodPost, path: "/api/v1/post", body: jsonBody(`{"title":"title","content":"content","status":1}`), auth: true, wantStatus: http.StatusOK, wantFields: []string{"postId"}},
		{id: "POST-GET-VALID", method: http.MethodGet, path: "/api/v1/post/11", routePath: "/api/v1/post/:postId", auth: true, wantStatus: http.StatusOK, wantFields: []string{"id", "authorId", "title", "content"}},
		{id: "POST-UPDATE-VALID", method: http.MethodPut, path: "/api/v1/post/11", routePath: "/api/v1/post/:postId", body: jsonBody(`{"title":"updated"}`), auth: true, wantStatus: http.StatusOK},
		{id: "POST-DELETE-VALID", method: http.MethodDelete, path: "/api/v1/post/11", routePath: "/api/v1/post/:postId", auth: true, wantStatus: http.StatusOK},
		{id: "COMMENT-CREATE-VALID", method: http.MethodPost, path: "/api/v1/comment", body: jsonBody(`{"postId":11,"content":"comment"}`), auth: true, wantStatus: http.StatusOK, wantFields: []string{"commentId"}},
		{id: "COMMENT-DELETE-VALID", method: http.MethodDelete, path: "/api/v1/comment/21", routePath: "/api/v1/comment/:commentId", auth: true, wantStatus: http.StatusOK},
		{id: "COMMENT-LIST-VALID", method: http.MethodGet, path: "/api/v1/comments/11?page=1&pageSize=20&sortBy=1", routePath: "/api/v1/comments/:postId", auth: true, wantStatus: http.StatusOK, wantFields: []string{"list", "total", "page", "pageSize"}},
		{id: "LIKE-VALID", method: http.MethodPost, path: "/api/v1/like", body: jsonBody(`{"targetId":11,"targetType":1}`), auth: true, wantStatus: http.StatusOK},
		{id: "UNLIKE-VALID", method: http.MethodDelete, path: "/api/v1/like", body: jsonBody(`{"targetId":11,"targetType":1}`), auth: true, wantStatus: http.StatusOK},
		{id: "FAVORITE-VALID", method: http.MethodPost, path: "/api/v1/favorite", body: jsonBody(`{"postId":11}`), auth: true, wantStatus: http.StatusOK},
		{id: "UNFAVORITE-VALID", method: http.MethodDelete, path: "/api/v1/favorite", body: jsonBody(`{"postId":11}`), auth: true, wantStatus: http.StatusOK},
		{id: "MEDIA-IMAGE-VALID", method: http.MethodPost, path: "/api/v1/media/image", body: imageBody, auth: true, wantStatus: http.StatusOK, wantFields: []string{"mediaId", "url", "thumbnailUrl"}},
	}

	decisions := append([]restDecision{}, successes...)
	for _, success := range successes {
		if success.auth {
			unauthorized := success
			unauthorized.id += "-NO-TOKEN"
			unauthorized.auth = false
			unauthorized.wantStatus = http.StatusUnauthorized
			unauthorized.wantCode = errx.LoginRequired
			unauthorized.wantFields = nil
			decisions = append(decisions, unauthorized)
		}
	}
	decisions = append(decisions,
		restDecision{id: "AUTH-REGISTER-EMPTY", method: http.MethodPost, path: "/api/v1/auth/register", body: jsonBody(`{}`), wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "AUTH-LOGIN-EMPTY", method: http.MethodPost, path: "/api/v1/auth/login", body: jsonBody(`{}`), wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "AUTH-CODE-BAD-PHONE", method: http.MethodPost, path: "/api/v1/auth/verify-code", body: jsonBody(`{"phone":"123","type":1}`), wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "POST-LIST-VALID-TOKEN", method: http.MethodGet, path: "/api/v1/posts", auth: true, wantStatus: http.StatusOK, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateAuthenticated}},
		restDecision{id: "POST-LIST-INVALID-TOKEN", method: http.MethodGet, path: "/api/v1/posts", headerToken: "invalid", wantStatus: http.StatusOK, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateInvalid}},
		restDecision{id: "POST-LIST-EXPIRED-TOKEN", method: http.MethodGet, path: "/api/v1/posts", headerToken: expiredToken, wantStatus: http.StatusOK, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateExpired}},
		restDecision{id: "POST-CREATE-INVALID-TOKEN", method: http.MethodPost, path: "/api/v1/post", body: jsonBody(`{"title":"title","content":"content"}`), headerToken: "invalid", wantStatus: http.StatusUnauthorized, wantCode: errx.LoginRequired},
		restDecision{id: "POST-CREATE-EXPIRED-TOKEN", method: http.MethodPost, path: "/api/v1/post", body: jsonBody(`{"title":"title","content":"content"}`), headerToken: expiredToken, wantStatus: http.StatusUnauthorized, wantCode: errx.LoginRequired},
		restDecision{id: "USER-GET-BAD-PATH", method: http.MethodGet, path: "/api/v1/user/not-a-number", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "POST-LIST-BAD-QUERY", method: http.MethodGet, path: "/api/v1/posts?page=not-a-number", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "USER-POSTS-BAD-QUERY", method: http.MethodGet, path: "/api/v1/users/2/posts?page=not-a-number", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "USER-FAVORITES-BAD-QUERY", method: http.MethodGet, path: "/api/v1/users/2/favorites?pageSize=not-a-number", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "USER-PROFILE-MALFORMED", method: http.MethodPut, path: "/api/v1/user/profile", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "USER-FOLLOW-MALFORMED", method: http.MethodPost, path: "/api/v1/user/follow", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "USER-UNFOLLOW-MALFORMED", method: http.MethodDelete, path: "/api/v1/user/follow", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "POST-CREATE-MALFORMED", method: http.MethodPost, path: "/api/v1/post", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "POST-GET-BAD-PATH", method: http.MethodGet, path: "/api/v1/post/not-a-number", auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "POST-UPDATE-BAD-PATH", method: http.MethodPut, path: "/api/v1/post/not-a-number", body: jsonBody(`{}`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "POST-DELETE-BAD-PATH", method: http.MethodDelete, path: "/api/v1/post/not-a-number", auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "COMMENT-CREATE-MALFORMED", method: http.MethodPost, path: "/api/v1/comment", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "COMMENT-DELETE-BAD-PATH", method: http.MethodDelete, path: "/api/v1/comment/not-a-number", auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "COMMENT-LIST-BAD-PATH", method: http.MethodGet, path: "/api/v1/comments/not-a-number", auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "LIKE-MALFORMED", method: http.MethodPost, path: "/api/v1/like", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "UNLIKE-MALFORMED", method: http.MethodDelete, path: "/api/v1/like", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "FAVORITE-MALFORMED", method: http.MethodPost, path: "/api/v1/favorite", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "UNFAVORITE-MALFORMED", method: http.MethodDelete, path: "/api/v1/favorite", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "MEDIA-IMAGE-NOT-MULTIPART", method: http.MethodPost, path: "/api/v1/media/image", body: plainBody("not-multipart"), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.FileTooLarge},
	)

	client := &http.Client{Timeout: 2 * time.Second}
	for _, decision := range decisions {
		t.Run(decision.id, func(t *testing.T) {
			var body io.Reader
			var contentType string
			if decision.body != nil {
				body, contentType = decision.body(t)
			}
			req, err := http.NewRequest(decision.method, baseURL+decision.path, body)
			if err != nil {
				t.Fatal(err)
			}
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			if decision.auth {
				req.Header.Set("Authorization", "Bearer "+token)
			} else if decision.headerToken != "" {
				req.Header.Set("Authorization", "Bearer "+decision.headerToken)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			payload, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != decision.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", resp.StatusCode, decision.wantStatus, payload)
			}
			for header, want := range decision.wantHeaders {
				if got := resp.Header.Get(header); got != want {
					t.Errorf("header %s=%q want=%q", header, got, want)
				}
			}
			if decision.wantCode != 0 {
				var envelope struct {
					Code int `json:"code"`
				}
				if err := json.Unmarshal(payload, &envelope); err != nil {
					t.Fatalf("decode error envelope: %v; body=%s", err, payload)
				}
				if envelope.Code != decision.wantCode {
					t.Fatalf("code=%d want=%d body=%s", envelope.Code, decision.wantCode, payload)
				}
			} else if decision.wantStatus == http.StatusOK {
				var document map[string]any
				if err := json.Unmarshal(payload, &document); err != nil {
					t.Fatalf("decode success response: %v; body=%s", err, payload)
				}
				for _, field := range decision.wantFields {
					if _, ok := document[field]; !ok {
						t.Errorf("response missing field %q; body=%s", field, payload)
					}
				}
			}
		})
	}

	if len(successes) != 23 {
		t.Fatalf("route inventory drift: got %d success rules, want 23", len(successes))
	}
	coveredRoutes := make(map[string]struct{}, len(successes))
	for _, success := range successes {
		path := success.routePath
		if path == "" {
			parsed, err := url.Parse(success.path)
			if err != nil {
				t.Fatal(err)
			}
			path = parsed.Path
		}
		coveredRoutes[success.method+" "+path] = struct{}{}
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if _, ok := coveredRoutes[key]; !ok {
			t.Errorf("registered route has no success decision: %s", key)
		}
		delete(coveredRoutes, key)
	}
	for key := range coveredRoutes {
		t.Errorf("decision references an unregistered route: %s", key)
	}
}
