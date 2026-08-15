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
	"strings"
	"testing"
	"time"

	"errx"
	"esx/app/assistant/rpc/assistantservice"
	assistantpb "esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/behavior/rpc/behaviorservice"
	"esx/app/content/rpc/contentservice"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/app/feed/rpc/feedservice"
	feedpb "esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/media/rpc/mediaservice"
	mediapb "esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/app/message/rpc/messageservice"
	messagepb "esx/app/message/rpc/xiaobaihe/message/pb"
	"esx/app/search/rpc/searchservice"
	"gateway/internal/config"
	"gateway/internal/handler"
	"gateway/internal/httpxconfig"
	gatewaymiddleware "gateway/internal/middleware"
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
func (contractUserService) BatchGetUsers(_ context.Context, in *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
	users := make([]*userpb.UserInfo, 0, len(in.UserIds))
	for _, userID := range in.UserIds {
		users = append(users, &userpb.UserInfo{Id: userID, Username: "author", Nickname: "Author Name", AvatarUrl: "https://media/avatar.png"})
	}
	return &userservice.BatchGetUsersResp{Users: users}, nil
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
func (contractUserService) SendVerifyCode(_ context.Context, in *userservice.SendVerifyCodeReq, _ ...grpc.CallOption) (*userservice.SendVerifyCodeResp, error) {
	if in.Phone == "13999999999" {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &userservice.SendVerifyCodeResp{}, nil
}

func (contractUserService) GetPersonalizationPreference(_ context.Context, in *userservice.GetPersonalizationPreferenceReq, _ ...grpc.CallOption) (*userservice.GetPersonalizationPreferenceResp, error) {
	if in.UserId == 999 {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &userservice.GetPersonalizationPreferenceResp{Enabled: true}, nil
}

func (contractUserService) SetPersonalizationPreference(_ context.Context, in *userservice.SetPersonalizationPreferenceReq, _ ...grpc.CallOption) (*userservice.SetPersonalizationPreferenceResp, error) {
	if in.UserId == 999 {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &userservice.SetPersonalizationPreferenceResp{}, nil
}

type contractContentService struct{ contentservice.ContentService }

func contractPost() *contentpb.PostInfo {
	return &contentpb.PostInfo{Id: 11, AuthorId: 1, Title: "title", Content: "content", Status: 1}
}
func (contractContentService) CreatePost(_ context.Context, in *contentservice.CreatePostReq, _ ...grpc.CallOption) (*contentservice.CreatePostResp, error) {
	if in.Title == "conflict-title" {
		// CORE-051：版本冲突必须可区分（REST 409 + 业务码）。
		return nil, errx.NewWithCode(errx.ContentVersionConflict)
	}
	return &contentservice.CreatePostResp{PostId: 11}, nil
}
func (contractContentService) GetPost(context.Context, *contentservice.GetPostReq, ...grpc.CallOption) (*contentservice.GetPostResp, error) {
	return &contentservice.GetPostResp{Post: contractPost()}, nil
}
func (contractContentService) UpdatePost(_ context.Context, in *contentservice.UpdatePostReq, _ ...grpc.CallOption) (*contentservice.UpdatePostResp, error) {
	if in.ExpectedRevision == 999 {
		// CORE-013：版本冲突必须返回 409 与 ContentVersionConflict。
		return nil, errx.NewWithCode(errx.ContentVersionConflict)
	}
	return &contentservice.UpdatePostResp{}, nil
}
func (contractContentService) DeletePost(context.Context, *contentservice.DeletePostReq, ...grpc.CallOption) (*contentservice.DeletePostResp, error) {
	return &contentservice.DeletePostResp{}, nil
}
func (contractContentService) GetPostList(_ context.Context, in *contentservice.GetPostListReq, _ ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	if in.Page == 999 {
		return nil, errx.NewWithCode(errx.SystemError)
	}
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
func (contractInteractionService) BatchCheckLiked(_ context.Context, in *interactionservice.BatchCheckLikedReq, _ ...grpc.CallOption) (*interactionservice.BatchCheckLikedResp, error) {
	results := make(map[int64]bool, len(in.TargetIds))
	for _, targetID := range in.TargetIds {
		results[targetID] = true
	}
	return &interactionservice.BatchCheckLikedResp{Results: results}, nil
}

func (contractInteractionService) BatchCheckFavorited(_ context.Context, in *interactionservice.BatchCheckFavoritedReq, _ ...grpc.CallOption) (*interactionservice.BatchCheckFavoritedResp, error) {
	results := make(map[int64]bool, len(in.PostIds))
	for _, postID := range in.PostIds {
		results[postID] = true
	}
	return &interactionservice.BatchCheckFavoritedResp{Results: results}, nil
}

type contractUploadStream struct {
	grpc.ClientStream
	failUserID int64
}

func (s *contractUploadStream) Send(in *mediapb.UploadImageReq) error {
	if meta := in.GetMeta(); meta != nil && meta.UserId == s.failUserID {
		s.failUserID = -1 // 已触发失败，CloseAndRecv 返回错误
	}
	return nil
}
func (s *contractUploadStream) CloseAndRecv() (*mediapb.UploadImageResp, error) {
	if s.failUserID == -1 {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &mediapb.UploadImageResp{Media: &mediapb.MediaInfo{Id: 31, Url: "https://media/image.png"}}, nil
}

type contractMediaService struct{ mediaservice.MediaService }

func (contractMediaService) UploadImage(context.Context, ...grpc.CallOption) (mediapb.MediaService_UploadImageClient, error) {
	return &contractUploadStream{failUserID: 999}, nil
}

type contractBehaviorService struct {
	behaviorservice.BehaviorService
}

func (contractBehaviorService) RecordEvents(_ context.Context, in *behaviorservice.RecordEventsReq, _ ...grpc.CallOption) (*behaviorservice.RecordEventsResp, error) {
	results := make([]*behaviorservice.RecordEventResult, 0, len(in.Events))
	for index, event := range in.Events {
		results = append(results, &behaviorservice.RecordEventResult{ClientEventId: event.ClientEventId, EventId: int64(index + 1), Accepted: true})
	}
	return &behaviorservice.RecordEventsResp{Results: results, AcceptedCount: int32(len(results))}, nil
}

type contractFeedService struct{ feedservice.FeedService }

func (contractFeedService) GetFollowFeed(context.Context, *feedservice.GetFollowFeedReq, ...grpc.CallOption) (*feedservice.GetFollowFeedResp, error) {
	return &feedservice.GetFollowFeedResp{
		Items: []*feedpb.FeedItem{{
			PostId: 11, AuthorId: 1, CreatedAt: 100, FeedType: 1,
			Title: "follow title", Content: "follow content", Images: []string{"follow.png"}, Tags: []string{"follow"},
			ViewCount: 10, LikeCount: 9, CommentCount: 8, FavoriteCount: 7,
		}},
	}, nil
}

func (contractFeedService) GetRecommendFeed(_ context.Context, in *feedservice.GetRecommendFeedReq, _ ...grpc.CallOption) (*feedservice.GetRecommendFeedResp, error) {
	if in.RequestId == "rpc-fail" {
		return nil, context.DeadlineExceeded
	}
	return &feedservice.GetRecommendFeedResp{
		Items: []*feedpb.FeedItem{{
			PostId: 11, AuthorId: 1, CreatedAt: 100, FeedType: 2,
			Title: "recommend title", Content: "recommend content", Images: []string{"recommend.png"}, Tags: []string{"recommend"},
			ViewCount: 20, LikeCount: 19, CommentCount: 18, FavoriteCount: 17,
			Score: 0.9, Reason: "relevant", RecallSource: "itemcf", Position: 1,
		}},
		RequestId: in.RequestId,
	}, nil
}

type contractSearchService struct{ searchservice.SearchService }

func (contractSearchService) Search(context.Context, *searchservice.SearchReq, ...grpc.CallOption) (*searchservice.SearchResp, error) {
	return &searchservice.SearchResp{
		Posts: []*searchservice.PostSearchResult{{Id: 11, Title: "title"}},
		Users: []*searchservice.UserSearchResult{{Id: 2, Username: "user"}},
		Tags:  []*searchservice.TagSearchResult{{Name: "tag", PostCount: 1}},
	}, nil
}
func (contractSearchService) SearchUsers(context.Context, *searchservice.SearchUsersReq, ...grpc.CallOption) (*searchservice.SearchUsersResp, error) {
	return &searchservice.SearchUsersResp{Users: []*searchservice.UserSearchResult{{Id: 2, Username: "user"}}, Total: 1}, nil
}
func (contractSearchService) SearchTags(context.Context, *searchservice.SearchTagsReq, ...grpc.CallOption) (*searchservice.SearchTagsResp, error) {
	return &searchservice.SearchTagsResp{Tags: []*searchservice.TagSearchResult{{Name: "tag", PostCount: 1}}}, nil
}

type contractMessageService struct{ messageservice.MessageService }

func (contractMessageService) GetConversations(context.Context, *messageservice.GetConversationsReq, ...grpc.CallOption) (*messageservice.GetConversationsResp, error) {
	return &messageservice.GetConversationsResp{
		Conversations: []*messagepb.ConversationInfo{{Id: 41, TargetUserId: 2, TargetUserName: "user", LastMessage: "hello"}},
		Total:         1,
	}, nil
}
func (contractMessageService) GetMessages(context.Context, *messageservice.GetMessagesReq, ...grpc.CallOption) (*messageservice.GetMessagesResp, error) {
	return &messageservice.GetMessagesResp{
		Messages: []*messagepb.MessageInfo{{Id: 51, ConversationId: 41, SenderId: 1, ReceiverId: 2, Content: "hello", MsgType: 1}},
	}, nil
}
func (contractMessageService) SendMessage(context.Context, *messageservice.SendMessageReq, ...grpc.CallOption) (*messageservice.SendMessageResp, error) {
	return &messageservice.SendMessageResp{MessageId: 51}, nil
}
func (contractMessageService) MarkRead(context.Context, *messageservice.MarkReadReq, ...grpc.CallOption) (*messageservice.MarkReadResp, error) {
	return &messageservice.MarkReadResp{}, nil
}
func (contractMessageService) GetUnreadCount(_ context.Context, in *messageservice.GetUnreadCountReq, _ ...grpc.CallOption) (*messageservice.GetUnreadCountResp, error) {
	if in.UserId == 999 {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &messageservice.GetUnreadCountResp{MessageUnread: 3, NotificationUnread: 4}, nil
}

type contractAssistantStream struct {
	grpc.ClientStream
	ctx    context.Context
	events []*assistantpb.ChatEvent
	index  int
}

func (s *contractAssistantStream) Context() context.Context { return s.ctx }

func (s *contractAssistantStream) Recv() (*assistantpb.ChatEvent, error) {
	if s.index >= len(s.events) {
		return nil, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

type contractAssistantService struct {
	assistantservice.AssistantService
}

func (contractAssistantService) Chat(ctx context.Context, in *assistantservice.ChatReq, _ ...grpc.CallOption) (assistantpb.AssistantService_ChatClient, error) {
	return &contractAssistantStream{ctx: ctx, events: []*assistantpb.ChatEvent{
		{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_TOKEN, Text: "answer", ConversationId: in.ConversationId},
		{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_SOURCE, Source: &assistantpb.SourceReference{SourceType: "post", SourceId: "11", Title: "title"}, ConversationId: in.ConversationId},
		{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_DONE, ConversationId: in.ConversationId},
	}}, nil
}

type restDecision struct {
	id             string
	method         string
	path           string
	routePath      string
	body           func(t *testing.T) (io.Reader, string)
	auth           bool
	headerToken    string
	wantStatus     int
	wantCode       int
	wantFields     []string
	wantItemFields []string
	wantHeaders    map[string]string
	wantSSE        bool
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
	behaviorAccepted := gatewaymiddleware.NewBehaviorAcceptedMiddleware()
	ctx := &svc.ServiceContext{
		Config:             cfg,
		UserService:        contractUserService{},
		ContentService:     contractContentService{},
		InteractionService: contractInteractionService{},
		MediaService:       contractMediaService{},
		BehaviorService:    contractBehaviorService{},
		FeedService:        contractFeedService{},
		MessageService:     contractMessageService{},
		SearchService:      contractSearchService{},
		AssistantService:   contractAssistantService{},
		OptionalAuth:       optionalAuth.Handle,
		BehaviorAccepted:   behaviorAccepted.Handle,
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
	failToken, err := jwtx.GenerateToken(999, "fail-user", jwtx.JwtConfig{AccessSecret: contractSecret, AccessExpire: 3600})
	if err != nil {
		t.Fatal(err)
	}

	feedItemFields := []string{
		"postId", "authorId", "authorName", "authorAvatar", "createdAt", "feedType", "title", "content", "images", "tags",
		"viewCount", "likeCount", "commentCount", "favoriteCount", "isLiked",
	}
	successes := []restDecision{
		{id: "HEALTH-VALID", method: http.MethodGet, path: "/api/v1/health", wantStatus: http.StatusOK, wantFields: []string{"status"}},
		{id: "HEALTH-READY-VALID", method: http.MethodGet, path: "/api/v1/health/ready", wantStatus: http.StatusOK, wantFields: []string{"status", "dependencies"}},
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
		{id: "POST-GET-ANON", method: http.MethodGet, path: "/api/v1/post/11", routePath: "/api/v1/post/:postId", wantStatus: http.StatusOK, wantFields: []string{"id", "authorId", "title", "content"}, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateAnonymous}},
		{id: "COMMENT-CREATE-VALID", method: http.MethodPost, path: "/api/v1/comment", body: jsonBody(`{"postId":11,"content":"comment"}`), auth: true, wantStatus: http.StatusOK, wantFields: []string{"commentId"}},
		{id: "COMMENT-DELETE-VALID", method: http.MethodDelete, path: "/api/v1/comment/21", routePath: "/api/v1/comment/:commentId", auth: true, wantStatus: http.StatusOK},
		{id: "COMMENT-LIST-ANON", method: http.MethodGet, path: "/api/v1/comments/11?page=1&pageSize=20&sortBy=1", routePath: "/api/v1/comments/:postId", wantStatus: http.StatusOK, wantFields: []string{"list", "total", "page", "pageSize"}, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateAnonymous}},
		{id: "LIKE-VALID", method: http.MethodPost, path: "/api/v1/like", body: jsonBody(`{"targetId":11,"targetType":1}`), auth: true, wantStatus: http.StatusOK},
		{id: "UNLIKE-VALID", method: http.MethodDelete, path: "/api/v1/like", body: jsonBody(`{"targetId":11,"targetType":1}`), auth: true, wantStatus: http.StatusOK},
		{id: "FAVORITE-VALID", method: http.MethodPost, path: "/api/v1/favorite", body: jsonBody(`{"postId":11}`), auth: true, wantStatus: http.StatusOK},
		{id: "UNFAVORITE-VALID", method: http.MethodDelete, path: "/api/v1/favorite", body: jsonBody(`{"postId":11}`), auth: true, wantStatus: http.StatusOK},
		{id: "MEDIA-IMAGE-VALID", method: http.MethodPost, path: "/api/v1/media/image", body: imageBody, auth: true, wantStatus: http.StatusOK, wantFields: []string{"mediaId", "url", "thumbnailUrl"}},
		{id: "BEHAVIOR-EVENTS-ANON", method: http.MethodPost, path: "/api/v2/behavior/events", body: jsonBody(`{"anonymousId":"device-1","events":[{"clientEventId":"event-1","occurredAt":1720000000000,"action":"click","targetId":11,"targetType":"post"}]}`), wantStatus: http.StatusAccepted, wantFields: []string{"results", "acceptedCount", "rejectedCount"}, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateAnonymous}},
		{id: "FEED-FOLLOW-VALID", method: http.MethodGet, path: "/api/v2/feed/follow?pageSize=20", auth: true, wantStatus: http.StatusOK, wantFields: []string{"items", "hasMore", "nextCursorCreatedAt", "nextCursorPostId"}, wantItemFields: feedItemFields},
		{id: "FEED-RECOMMEND-ANON", method: http.MethodGet, path: "/api/v2/feed/recommend?anonymousId=device-1&requestId=request-1&pageSize=20", wantStatus: http.StatusOK, wantFields: []string{"items", "nextCursor", "hasMore", "requestId"}, wantItemFields: feedItemFields, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateAnonymous}},
		{id: "SEARCH-VALID", method: http.MethodGet, path: "/api/v2/search?keyword=go&page=1&pageSize=20", wantStatus: http.StatusOK, wantFields: []string{"posts", "users", "tags"}},
		{id: "SEARCH-USERS-VALID", method: http.MethodGet, path: "/api/v2/search/users?keyword=user&page=1&pageSize=20", wantStatus: http.StatusOK, wantFields: []string{"users", "total"}},
		{id: "SEARCH-TAGS-VALID", method: http.MethodGet, path: "/api/v2/search/tags?keyword=tag&limit=20", wantStatus: http.StatusOK, wantFields: []string{"tags"}},
		{id: "MESSAGE-CONVERSATIONS-VALID", method: http.MethodGet, path: "/api/v2/messages/conversations?page=1&pageSize=20", auth: true, wantStatus: http.StatusOK, wantFields: []string{"conversations", "total"}},
		{id: "MESSAGE-LIST-VALID", method: http.MethodGet, path: "/api/v2/messages/conversations/41?pageSize=20", routePath: "/api/v2/messages/conversations/:id", auth: true, wantStatus: http.StatusOK, wantFields: []string{"messages", "hasMore"}},
		{id: "MESSAGE-SEND-VALID", method: http.MethodPost, path: "/api/v2/messages", body: jsonBody(`{"receiverId":2,"content":"hello","msgType":1,"idempotencyKey":"send-1"}`), auth: true, wantStatus: http.StatusOK, wantFields: []string{"messageId"}},
		{id: "MESSAGE-MARK-READ-VALID", method: http.MethodPost, path: "/api/v2/messages/conversations/41/read", routePath: "/api/v2/messages/conversations/:id/read", auth: true, wantStatus: http.StatusOK},
		{id: "MESSAGE-UNREAD-VALID", method: http.MethodGet, path: "/api/v2/messages/unread", auth: true, wantStatus: http.StatusOK, wantFields: []string{"messageUnread", "notificationUnread"}},
		{id: "PERSONALIZATION-GET-VALID", method: http.MethodGet, path: "/api/v2/me/personalization", auth: true, wantStatus: http.StatusOK, wantFields: []string{"enabled", "optedOutAt"}},
		{id: "PERSONALIZATION-PUT-VALID", method: http.MethodPut, path: "/api/v2/me/personalization", body: jsonBody(`{"enabled":false}`), auth: true, wantStatus: http.StatusOK},
		{id: "POST-CREATE-V2-VALID", method: http.MethodPost, path: "/api/v2/post", body: jsonBody(`{"title":"title","content":"content","status":1}`), auth: true, wantStatus: http.StatusOK, wantFields: []string{"postId"}},
		{id: "POST-UPDATE-V2-VALID", method: http.MethodPut, path: "/api/v2/post/11", routePath: "/api/v2/post/:postId", body: jsonBody(`{"title":"updated","expectedRevision":1}`), auth: true, wantStatus: http.StatusOK},
		{id: "POST-DELETE-V2-VALID", method: http.MethodDelete, path: "/api/v2/post/11", routePath: "/api/v2/post/:postId", body: jsonBody(`{"expectedRevision":1}`), auth: true, wantStatus: http.StatusOK},
		{id: "ASSISTANT-CHAT-VALID", method: http.MethodPost, path: "/api/v2/assistant/chat", body: jsonBody(`{"conversationId":"conversation-1","message":"hello","requestId":"request-1"}`), auth: true, wantStatus: http.StatusOK, wantSSE: true},
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
		restDecision{id: "POST-GET-AUTHENTICATED", method: http.MethodGet, path: "/api/v1/post/11", auth: true, wantStatus: http.StatusOK, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateAuthenticated}},
		restDecision{id: "POST-GET-INVALID-TOKEN", method: http.MethodGet, path: "/api/v1/post/11", headerToken: "invalid", wantStatus: http.StatusOK, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateInvalid}},
		restDecision{id: "COMMENT-LIST-AUTHENTICATED", method: http.MethodGet, path: "/api/v1/comments/11?page=1&pageSize=20&sortBy=1", auth: true, wantStatus: http.StatusOK, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateAuthenticated}},
		restDecision{id: "COMMENT-LIST-EXPIRED-TOKEN", method: http.MethodGet, path: "/api/v1/comments/11?page=1&pageSize=20&sortBy=1", headerToken: expiredToken, wantStatus: http.StatusOK, wantHeaders: map[string]string{middleware.AuthStateHeader: middleware.AuthStateExpired}},
		restDecision{id: "USER-GET-BAD-PATH", method: http.MethodGet, path: "/api/v1/user/not-a-number", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "POST-LIST-BAD-QUERY", method: http.MethodGet, path: "/api/v1/posts?page=not-a-number", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "USER-POSTS-BAD-QUERY", method: http.MethodGet, path: "/api/v1/users/2/posts?page=not-a-number", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "USER-FAVORITES-BAD-QUERY", method: http.MethodGet, path: "/api/v1/users/2/favorites?pageSize=not-a-number", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "USER-PROFILE-MALFORMED", method: http.MethodPut, path: "/api/v1/user/profile", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "USER-FOLLOW-MALFORMED", method: http.MethodPost, path: "/api/v1/user/follow", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "USER-UNFOLLOW-MALFORMED", method: http.MethodDelete, path: "/api/v1/user/follow", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "POST-GET-BAD-PATH", method: http.MethodGet, path: "/api/v1/post/not-a-number", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "COMMENT-CREATE-MALFORMED", method: http.MethodPost, path: "/api/v1/comment", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "COMMENT-DELETE-BAD-PATH", method: http.MethodDelete, path: "/api/v1/comment/not-a-number", auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "COMMENT-LIST-BAD-PATH", method: http.MethodGet, path: "/api/v1/comments/not-a-number", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "LIKE-MALFORMED", method: http.MethodPost, path: "/api/v1/like", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "UNLIKE-MALFORMED", method: http.MethodDelete, path: "/api/v1/like", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "FAVORITE-MALFORMED", method: http.MethodPost, path: "/api/v1/favorite", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "UNFAVORITE-MALFORMED", method: http.MethodDelete, path: "/api/v1/favorite", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "MEDIA-IMAGE-NOT-MULTIPART", method: http.MethodPost, path: "/api/v1/media/image", body: plainBody("not-multipart"), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "BEHAVIOR-EVENTS-MALFORMED", method: http.MethodPost, path: "/api/v2/behavior/events", body: jsonBody(`{`), wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "FEED-FOLLOW-BAD-QUERY", method: http.MethodGet, path: "/api/v2/feed/follow?pageSize=bad", auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "FEED-RECOMMEND-BAD-QUERY", method: http.MethodGet, path: "/api/v2/feed/recommend?anonymousId=device-1&requestId=request-1&pageSize=bad", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "FEED-RECOMMEND-RPC-FAILURE", method: http.MethodGet, path: "/api/v2/feed/recommend?anonymousId=device-1&requestId=rpc-fail&pageSize=20", wantStatus: http.StatusInternalServerError, wantCode: errx.SystemError},
		restDecision{id: "AUTH-CODE-RPC-FAIL", method: http.MethodPost, path: "/api/v1/auth/verify-code", body: jsonBody(`{"phone":"13999999999","type":1}`), wantStatus: http.StatusInternalServerError, wantCode: errx.SystemError},
		restDecision{id: "PERSONALIZATION-GET-RPC-FAIL", method: http.MethodGet, path: "/api/v2/me/personalization", headerToken: failToken, wantStatus: http.StatusInternalServerError, wantCode: errx.SystemError},
		restDecision{id: "PERSONALIZATION-PUT-RPC-FAIL", method: http.MethodPut, path: "/api/v2/me/personalization", body: jsonBody(`{"enabled":false}`), headerToken: failToken, wantStatus: http.StatusInternalServerError, wantCode: errx.SystemError},
		restDecision{id: "MESSAGE-UNREAD-RPC-FAIL", method: http.MethodGet, path: "/api/v2/messages/unread", headerToken: failToken, wantStatus: http.StatusInternalServerError, wantCode: errx.SystemError},
		restDecision{id: "MEDIA-IMAGE-RPC-FAIL", method: http.MethodPost, path: "/api/v1/media/image", body: imageBody, headerToken: failToken, wantStatus: http.StatusInternalServerError, wantCode: errx.SystemError},
		restDecision{id: "POST-LIST-RPC-FAIL", method: http.MethodGet, path: "/api/v1/posts?page=999", wantStatus: http.StatusInternalServerError, wantCode: errx.SystemError},
		restDecision{id: "POST-UPDATE-V2-MISSING-REVISION", method: http.MethodPut, path: "/api/v2/post/11", routePath: "/api/v2/post/:postId", body: jsonBody(`{"title":"updated"}`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "POST-UPDATE-V2-ZERO-REVISION", method: http.MethodPut, path: "/api/v2/post/11", routePath: "/api/v2/post/:postId", body: jsonBody(`{"title":"updated","expectedRevision":0}`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "POST-UPDATE-V2-CONFLICT", method: http.MethodPut, path: "/api/v2/post/11", routePath: "/api/v2/post/:postId", body: jsonBody(`{"title":"updated","expectedRevision":999}`), auth: true, wantStatus: http.StatusConflict, wantCode: errx.ContentVersionConflict},
		restDecision{id: "POST-DELETE-V2-MISSING-REVISION", method: http.MethodDelete, path: "/api/v2/post/11", routePath: "/api/v2/post/:postId", auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "POST-DELETE-V2-ZERO-REVISION", method: http.MethodDelete, path: "/api/v2/post/11", routePath: "/api/v2/post/:postId", body: jsonBody(`{"expectedRevision":0}`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "SEARCH-BAD-QUERY", method: http.MethodGet, path: "/api/v2/search?keyword=go&page=bad", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "SEARCH-USERS-BAD-QUERY", method: http.MethodGet, path: "/api/v2/search/users?keyword=user&page=bad", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "SEARCH-TAGS-BAD-QUERY", method: http.MethodGet, path: "/api/v2/search/tags?keyword=tag&limit=bad", wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "MESSAGE-CONVERSATIONS-BAD-QUERY", method: http.MethodGet, path: "/api/v2/messages/conversations?page=bad", auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "MESSAGE-LIST-BAD-QUERY", method: http.MethodGet, path: "/api/v2/messages/conversations/41?pageSize=bad", routePath: "/api/v2/messages/conversations/:id", auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "MESSAGE-SEND-MALFORMED", method: http.MethodPost, path: "/api/v2/messages", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "MESSAGE-MARK-READ-BAD-PATH", method: http.MethodPost, path: "/api/v2/messages/conversations/not-a-number/read", auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
		restDecision{id: "ASSISTANT-CHAT-MALFORMED", method: http.MethodPost, path: "/api/v2/assistant/chat", body: jsonBody(`{`), auth: true, wantStatus: http.StatusBadRequest, wantCode: errx.ParamError},
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
			if decision.wantSSE {
				req.Header.Set("Accept", "text/event-stream")
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
			} else if decision.wantSSE {
				if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
					t.Fatalf("content-type=%q want text/event-stream", got)
				}
				body := string(payload)
				if !strings.Contains(body, `"type":"token"`) || !strings.Contains(body, `"type":"source"`) || !strings.Contains(body, `"type":"done"`) {
					t.Fatalf("missing SSE events: %s", payload)
				}
			} else if decision.wantStatus == http.StatusOK || decision.wantStatus == http.StatusAccepted {
				var document map[string]any
				if err := json.Unmarshal(payload, &document); err != nil {
					t.Fatalf("decode success response: %v; body=%s", err, payload)
				}
				for _, field := range decision.wantFields {
					if _, ok := document[field]; !ok {
						t.Errorf("response missing field %q; body=%s", field, payload)
					}
				}
				if len(decision.wantItemFields) > 0 {
					items, ok := document["items"].([]any)
					if !ok || len(items) == 0 {
						t.Fatalf("response has no contract item; body=%s", payload)
					}
					item, ok := items[0].(map[string]any)
					if !ok {
						t.Fatalf("response item is not an object; body=%s", payload)
					}
					for _, field := range decision.wantItemFields {
						if _, ok := item[field]; !ok {
							t.Errorf("response item missing field %q; body=%s", field, payload)
						}
					}
				}
			}
		})
	}

	if len(successes) != 38 {
		t.Fatalf("route inventory drift: got %d success rules, want 38", len(successes))
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
