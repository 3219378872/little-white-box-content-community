package feed

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"esx/app/feed/rpc/feedservice"
	feedpb "esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/interaction/rpc/interactionservice"
	userpb "esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"google.golang.org/grpc"
)

type fakeFeedService struct {
	feedservice.FeedService
	getFollowFeedFn    func(context.Context, *feedservice.GetFollowFeedReq, ...grpc.CallOption) (*feedservice.GetFollowFeedResp, error)
	getRecommendFeedFn func(context.Context, *feedservice.GetRecommendFeedReq, ...grpc.CallOption) (*feedservice.GetRecommendFeedResp, error)
}

func (f *fakeFeedService) GetFollowFeed(ctx context.Context, in *feedservice.GetFollowFeedReq, opts ...grpc.CallOption) (*feedservice.GetFollowFeedResp, error) {
	return f.getFollowFeedFn(ctx, in, opts...)
}

func (f *fakeFeedService) GetRecommendFeed(ctx context.Context, in *feedservice.GetRecommendFeedReq, opts ...grpc.CallOption) (*feedservice.GetRecommendFeedResp, error) {
	return f.getRecommendFeedFn(ctx, in, opts...)
}

type fakeUserService struct {
	userservice.UserService
	batchGetUsersFn func(context.Context, *userservice.BatchGetUsersReq, ...grpc.CallOption) (*userservice.BatchGetUsersResp, error)
}

func (f *fakeUserService) BatchGetUsers(ctx context.Context, in *userservice.BatchGetUsersReq, opts ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
	return f.batchGetUsersFn(ctx, in, opts...)
}

type fakeInteractionService struct {
	interactionservice.InteractionService
	batchCheckLikedFn func(context.Context, *interactionservice.BatchCheckLikedReq, ...grpc.CallOption) (*interactionservice.BatchCheckLikedResp, error)
}

func (f *fakeInteractionService) BatchCheckLiked(ctx context.Context, in *interactionservice.BatchCheckLikedReq, opts ...grpc.CallOption) (*interactionservice.BatchCheckLikedResp, error) {
	return f.batchCheckLikedFn(ctx, in, opts...)
}

func TestGetFollowFeed_MapsRPCResponse(t *testing.T) {
	type requestContextKey struct{}
	ctxKey := requestContextKey{}
	ctx := context.WithValue(jwtx.WithUserIdContext(context.Background(), 42), ctxKey, "preserved")
	svcCtx := &svc.ServiceContext{
		FeedService: &fakeFeedService{
			getFollowFeedFn: func(gotCtx context.Context, in *feedservice.GetFollowFeedReq, _ ...grpc.CallOption) (*feedservice.GetFollowFeedResp, error) {
				assertPreservedContext(t, gotCtx, ctxKey)
				if in.UserId != 42 || in.CursorCreatedAt != 100 || in.CursorPostId != 200 || in.PageSize != 10 {
					t.Fatalf("unexpected feed request: %+v", in)
				}
				return &feedservice.GetFollowFeedResp{
					Items: []*feedpb.FeedItem{{
						PostId: 11, AuthorId: 12, CreatedAt: 13, FeedType: 1,
						Title: "title", Content: "content", Images: []string{"image.png"}, Tags: []string{"go"},
						ViewCount: 10, LikeCount: 9, CommentCount: 8, FavoriteCount: 7,
					}},
					HasMore:             true,
					NextCursorCreatedAt: 90,
					NextCursorPostId:    10,
				}, nil
			},
		},
		UserService: &fakeUserService{
			batchGetUsersFn: func(gotCtx context.Context, in *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
				assertPreservedContext(t, gotCtx, ctxKey)
				if !reflect.DeepEqual(in.UserIds, []int64{12}) {
					t.Fatalf("unexpected user request: %+v", in)
				}
				return &userservice.BatchGetUsersResp{Users: []*userpb.UserInfo{{
					Id: 12, Username: "fallback-name", Nickname: "  Display Name  ", AvatarUrl: "  https://avatar/12.png  ",
				}}}, nil
			},
		},
		InteractionService: &fakeInteractionService{
			batchCheckLikedFn: func(gotCtx context.Context, in *interactionservice.BatchCheckLikedReq, _ ...grpc.CallOption) (*interactionservice.BatchCheckLikedResp, error) {
				assertPreservedContext(t, gotCtx, ctxKey)
				if in.UserId != 42 || in.TargetType != postTargetType || !reflect.DeepEqual(in.TargetIds, []int64{11}) {
					t.Fatalf("unexpected interaction request: %+v", in)
				}
				return &interactionservice.BatchCheckLikedResp{Results: map[int64]bool{11: true}}, nil
			},
		},
	}

	resp, err := NewGetFollowFeedLogic(ctx, svcCtx).GetFollowFeed(&types.GetFollowFeedReq{
		CursorCreatedAt: 100,
		CursorPostId:    200,
		PageSize:        10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantItem := types.FeedItem{
		PostId: 11, AuthorId: 12, AuthorName: "Display Name", AuthorAvatar: "https://avatar/12.png", CreatedAt: 13, FeedType: 1,
		Title: "title", Content: "content", Images: []string{"image.png"}, Tags: []string{"go"},
		ViewCount: 10, LikeCount: 9, CommentCount: 8, FavoriteCount: 7, IsLiked: true,
	}
	if len(resp.Items) != 1 || !reflect.DeepEqual(resp.Items[0], wantItem) {
		t.Fatalf("unexpected items: %+v", resp.Items)
	}
	if !resp.HasMore || resp.NextCursorCreatedAt != 90 || resp.NextCursorPostId != 10 {
		t.Fatalf("unexpected cursor response: %+v", resp)
	}
}

func assertPreservedContext[K comparable](t *testing.T, ctx context.Context, key K) {
	t.Helper()
	if ctx.Value(key) != "preserved" {
		t.Fatal("request context was not propagated")
	}
}

func TestGetFollowFeed_RPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{FeedService: &fakeFeedService{
		getFollowFeedFn: func(context.Context, *feedservice.GetFollowFeedReq, ...grpc.CallOption) (*feedservice.GetFollowFeedResp, error) {
			return nil, context.DeadlineExceeded
		},
	}}

	_, err := NewGetFollowFeedLogic(jwtx.WithUserIdContext(context.Background(), 42), svcCtx).GetFollowFeed(&types.GetFollowFeedReq{PageSize: 20})
	if err == nil {
		t.Fatal("expected rpc error")
	}
}

func TestGetRecommendFeed_UsesFeedAndMapsMetadata(t *testing.T) {
	tests := []struct {
		name       string
		auth       bool
		wantUserID int64
	}{
		{name: "anonymous"},
		{name: "authenticated", auth: true, wantUserID: 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type requestContextKey struct{}
			ctxKey := requestContextKey{}
			ctx := context.WithValue(context.Background(), ctxKey, "preserved")
			if tt.auth {
				ctx = jwtx.WithUserIdContext(ctx, tt.wantUserID)
			}
			interactionCalled := false
			svcCtx := &svc.ServiceContext{
				FeedService: &fakeFeedService{
					getRecommendFeedFn: func(gotCtx context.Context, in *feedservice.GetRecommendFeedReq, _ ...grpc.CallOption) (*feedservice.GetRecommendFeedResp, error) {
						assertPreservedContext(t, gotCtx, ctxKey)
						if in.UserId != tt.wantUserID || in.AnonymousId != "device-1" || in.Scene != "home" || in.RequestId != "request-1" || in.Cursor != "cursor-1" || in.PageSize != 10 || in.ExperimentId != "exp-1" {
							t.Fatalf("unexpected rpc request: %+v", in)
						}
						return &feedservice.GetRecommendFeedResp{
							Items: []*feedpb.FeedItem{{
								PostId: 99, AuthorId: 42, CreatedAt: 1001, FeedType: 2,
								Title: "recommend title", Content: "recommend content", Images: []string{"recommend.png"}, Tags: []string{"recommend"},
								ViewCount: 20, LikeCount: 19, CommentCount: 18, FavoriteCount: 17,
								Score: 0.75, Reason: "similar", RecallSource: "itemcf",
								ModelVersion: "rank-v1", ExperimentId: "exp-1", Position: 3,
							}},
							NextCursor: "cursor-2",
							HasMore:    true,
							RequestId:  "request-1",
						}, nil
					},
				},
				UserService: &fakeUserService{
					batchGetUsersFn: func(gotCtx context.Context, in *userservice.BatchGetUsersReq, _ ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
						assertPreservedContext(t, gotCtx, ctxKey)
						if !reflect.DeepEqual(in.UserIds, []int64{42}) {
							t.Fatalf("unexpected user request: %+v", in)
						}
						return &userservice.BatchGetUsersResp{Users: []*userpb.UserInfo{{
							Id: 42, Username: "  fallback author  ", AvatarUrl: "https://avatar/42.png",
						}}}, nil
					},
				},
				InteractionService: &fakeInteractionService{
					batchCheckLikedFn: func(gotCtx context.Context, in *interactionservice.BatchCheckLikedReq, _ ...grpc.CallOption) (*interactionservice.BatchCheckLikedResp, error) {
						interactionCalled = true
						assertPreservedContext(t, gotCtx, ctxKey)
						if in.UserId != 42 || in.TargetType != postTargetType || !reflect.DeepEqual(in.TargetIds, []int64{99}) {
							t.Fatalf("unexpected interaction request: %+v", in)
						}
						return &interactionservice.BatchCheckLikedResp{Results: map[int64]bool{99: true}}, nil
					},
				},
			}

			resp, err := NewGetRecommendFeedLogic(ctx, svcCtx).GetRecommendFeed(&types.GetRecommendFeedReq{
				AnonymousId: "device-1", Scene: "home", RequestId: "request-1", SessionId: "session-1",
				Cursor: "cursor-1", PageSize: 10, ExperimentId: "exp-1",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(resp.Items) != 1 {
				t.Fatalf("unexpected response: %+v", resp)
			}
			item := resp.Items[0]
			wantItem := types.RecommendFeedItem{
				PostId: 99, AuthorId: 42, AuthorName: "fallback author", AuthorAvatar: "https://avatar/42.png", CreatedAt: 1001, FeedType: 2,
				Title: "recommend title", Content: "recommend content", Images: []string{"recommend.png"}, Tags: []string{"recommend"},
				ViewCount: 20, LikeCount: 19, CommentCount: 18, FavoriteCount: 17, IsLiked: tt.auth,
				Score: 0.75, Reason: "similar", RecallSource: "itemcf",
				ModelVersion: "rank-v1", ExperimentId: "exp-1", Position: 3,
			}
			if !reflect.DeepEqual(item, wantItem) {
				t.Fatalf("recommend metadata was not mapped: %+v", item)
			}
			if resp.NextCursor != "cursor-2" || !resp.HasMore || resp.RequestId != "request-1" {
				t.Fatalf("unexpected response metadata: %+v", resp)
			}
			if interactionCalled != tt.auth {
				t.Fatalf("interaction call=%v want=%v", interactionCalled, tt.auth)
			}
		})
	}
}

func TestGetRecommendFeed_FeedRPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{FeedService: &fakeFeedService{
		getRecommendFeedFn: func(context.Context, *feedservice.GetRecommendFeedReq, ...grpc.CallOption) (*feedservice.GetRecommendFeedResp, error) {
			return nil, context.DeadlineExceeded
		},
	}}

	_, err := NewGetRecommendFeedLogic(context.Background(), svcCtx).GetRecommendFeed(&types.GetRecommendFeedReq{
		AnonymousId: "device-1", Scene: "home", RequestId: "request-1", PageSize: 10,
	})
	if err == nil {
		t.Fatal("expected feed rpc error")
	}
}

func TestGetRecommendFeed_RejectsAnonymousWithoutIdentity(t *testing.T) {
	called := false
	svcCtx := &svc.ServiceContext{FeedService: &fakeFeedService{
		getRecommendFeedFn: func(context.Context, *feedservice.GetRecommendFeedReq, ...grpc.CallOption) (*feedservice.GetRecommendFeedResp, error) {
			called = true
			return &feedservice.GetRecommendFeedResp{}, nil
		},
	}}

	_, err := NewGetRecommendFeedLogic(context.Background(), svcCtx).GetRecommendFeed(&types.GetRecommendFeedReq{
		RequestId: "request-1", PageSize: 10,
	})
	if err == nil {
		t.Fatal("expected anonymous identity validation error")
	}
	if called {
		t.Fatal("feed rpc must not be called without an identity")
	}
}

func TestGetFollowFeed_UserEnrichmentFailure(t *testing.T) {
	tests := []struct {
		name string
		fn   func(context.Context, *userservice.BatchGetUsersReq, ...grpc.CallOption) (*userservice.BatchGetUsersResp, error)
	}{
		{
			name: "rpc error",
			fn: func(context.Context, *userservice.BatchGetUsersReq, ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
		{
			name: "nil response",
			fn: func(context.Context, *userservice.BatchGetUsersReq, ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
				return nil, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcCtx := &svc.ServiceContext{
				FeedService: &fakeFeedService{
					getFollowFeedFn: func(context.Context, *feedservice.GetFollowFeedReq, ...grpc.CallOption) (*feedservice.GetFollowFeedResp, error) {
						return &feedservice.GetFollowFeedResp{Items: []*feedpb.FeedItem{{PostId: 11, AuthorId: 12}}}, nil
					},
				},
				UserService: &fakeUserService{batchGetUsersFn: tt.fn},
			}

			_, err := NewGetFollowFeedLogic(jwtx.WithUserIdContext(context.Background(), 42), svcCtx).
				GetFollowFeed(&types.GetFollowFeedReq{PageSize: 20})
			if !errx.Is(err, errx.SystemError) {
				t.Fatalf("expected SystemError, got %v", err)
			}
		})
	}
}

func TestGetRecommendFeed_InteractionEnrichmentFailure(t *testing.T) {
	tests := []struct {
		name string
		fn   func(context.Context, *interactionservice.BatchCheckLikedReq, ...grpc.CallOption) (*interactionservice.BatchCheckLikedResp, error)
	}{
		{
			name: "rpc error",
			fn: func(context.Context, *interactionservice.BatchCheckLikedReq, ...grpc.CallOption) (*interactionservice.BatchCheckLikedResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
		{
			name: "nil response",
			fn: func(context.Context, *interactionservice.BatchCheckLikedReq, ...grpc.CallOption) (*interactionservice.BatchCheckLikedResp, error) {
				return nil, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcCtx := &svc.ServiceContext{
				FeedService: &fakeFeedService{
					getRecommendFeedFn: func(context.Context, *feedservice.GetRecommendFeedReq, ...grpc.CallOption) (*feedservice.GetRecommendFeedResp, error) {
						return &feedservice.GetRecommendFeedResp{Items: []*feedpb.FeedItem{{PostId: 11, AuthorId: 12}}}, nil
					},
				},
				UserService: &fakeUserService{
					batchGetUsersFn: func(context.Context, *userservice.BatchGetUsersReq, ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
						return &userservice.BatchGetUsersResp{Users: []*userpb.UserInfo{{Id: 12, Username: "author"}}}, nil
					},
				},
				InteractionService: &fakeInteractionService{batchCheckLikedFn: tt.fn},
			}

			_, err := NewGetRecommendFeedLogic(jwtx.WithUserIdContext(context.Background(), 42), svcCtx).
				GetRecommendFeed(&types.GetRecommendFeedReq{RequestId: "request-1", PageSize: 20})
			if !errx.Is(err, errx.SystemError) {
				t.Fatalf("expected SystemError, got %v", err)
			}
		})
	}
}

func TestFeedItems_JSONEnrichmentContract(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{
			name: "follow",
			value: types.FeedItem{
				AuthorName: "Alice", AuthorAvatar: "https://avatar/alice.png", IsLiked: true,
			},
		},
		{
			name: "recommend",
			value: types.RecommendFeedItem{
				AuthorName: "Bob", AuthorAvatar: "https://avatar/bob.png", IsLiked: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(payload, &document); err != nil {
				t.Fatal(err)
			}
			wantName := map[string]string{"follow": "Alice", "recommend": "Bob"}[tt.name]
			wantAvatar := map[string]string{
				"follow": "https://avatar/alice.png", "recommend": "https://avatar/bob.png",
			}[tt.name]
			if document["authorName"] != wantName || document["authorAvatar"] != wantAvatar || document["isLiked"] != (tt.name == "follow") {
				t.Fatalf("unexpected enrichment JSON: %s", payload)
			}
			for _, wrongKey := range []string{"author_name", "author_avatar", "is_liked"} {
				if _, ok := document[wrongKey]; ok {
					t.Fatalf("unexpected snake_case field %q: %s", wrongKey, payload)
				}
			}
		})
	}
}

func TestUniqueFeedIDs_DeduplicatesAndPreservesOrder(t *testing.T) {
	authors, posts := uniqueFeedIDs([]*feedpb.FeedItem{
		{PostId: 11, AuthorId: 2},
		nil,
		{PostId: 12, AuthorId: 1},
		{PostId: 11, AuthorId: 2},
		{PostId: 0, AuthorId: -1},
	})
	if !reflect.DeepEqual(authors, []int64{2, 1}) || !reflect.DeepEqual(posts, []int64{11, 12}) {
		t.Fatalf("unexpected unique IDs: authors=%v posts=%v", authors, posts)
	}
}
