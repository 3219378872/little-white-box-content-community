package agent

import (
	"context"
	"strings"
	"testing"

	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/interactionservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"

	"google.golang.org/grpc"
)

type fakeInteractionService struct {
	interactionservice.InteractionService
	favorites func(context.Context, *interactionservice.GetFavoriteListReq) (*interactionservice.GetFavoriteListResp, error)
	likes     func(context.Context, *interactionservice.GetLikeListReq) (*interactionservice.GetLikeListResp, error)
}

func (f *fakeInteractionService) GetFavoriteList(ctx context.Context, req *interactionservice.GetFavoriteListReq, _ ...grpc.CallOption) (*interactionservice.GetFavoriteListResp, error) {
	return f.favorites(ctx, req)
}

func (f *fakeInteractionService) GetLikeList(ctx context.Context, req *interactionservice.GetLikeListReq, _ ...grpc.CallOption) (*interactionservice.GetLikeListResp, error) {
	return f.likes(ctx, req)
}

type fakeUserService struct {
	userservice.UserService
	getUser     func(context.Context, *userservice.GetUserReq) (*userservice.GetUserResp, error)
	following   func(context.Context, *userservice.GetFollowingReq) (*userservice.GetFollowingResp, error)
	personalize func(context.Context, *userservice.GetPersonalizationPreferenceReq) (*userservice.GetPersonalizationPreferenceResp, error)
}

func (f *fakeUserService) GetUser(ctx context.Context, req *userservice.GetUserReq, _ ...grpc.CallOption) (*userservice.GetUserResp, error) {
	if f.getUser == nil {
		return &userservice.GetUserResp{}, nil
	}
	return f.getUser(ctx, req)
}

func (f *fakeUserService) GetFollowing(ctx context.Context, req *userservice.GetFollowingReq, _ ...grpc.CallOption) (*userservice.GetFollowingResp, error) {
	return f.following(ctx, req)
}

func (f *fakeUserService) GetPersonalizationPreference(ctx context.Context, req *userservice.GetPersonalizationPreferenceReq, _ ...grpc.CallOption) (*userservice.GetPersonalizationPreferenceResp, error) {
	if f.personalize == nil {
		return &userservice.GetPersonalizationPreferenceResp{Enabled: true}, nil
	}
	return f.personalize(ctx, req)
}

func publishedBackfill(ids ...int64) func(context.Context, *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
	allowed := map[int64]struct{}{}
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	return func(_ context.Context, req *contentservice.GetPostsByIdsReq) (*contentservice.GetPostsByIdsResp, error) {
		out := []*contentservice.PostInfo{}
		for _, id := range req.PostIds {
			status := int32(0)
			title := "draft"
			if _, ok := allowed[id]; ok {
				status = 1
				title = "t"
			}
			out = append(out, &contentservice.PostInfo{Id: id, Title: title, Content: "c", Status: status, Revision: 1})
		}
		return &contentservice.GetPostsByIdsResp{Posts: out}, nil
	}
}

func TestGetMyFavoritesBackfillsPublishedAndIgnoresForeignUser(t *testing.T) {
	var gotUserID int64
	interaction := &fakeInteractionService{
		favorites: func(_ context.Context, req *interactionservice.GetFavoriteListReq) (*interactionservice.GetFavoriteListResp, error) {
			gotUserID = req.UserId
			return &interactionservice.GetFavoriteListResp{PostIds: []int64{11, 12}, Total: 2}, nil
		},
	}
	content := &fakeContentService{postsByIDs: publishedBackfill(11)}
	registry, err := NewToolRegistry(Clients{Interaction: interaction, Content: content}, []string{ToolGetMyFavorites})
	if err != nil {
		t.Fatal(err)
	}
	text, sources, err := registry.Call(context.Background(), &Session{UserID: 7}, ToolGetMyFavorites, "c1", `{"user_id":99,"page":0}`)
	if err != nil {
		t.Fatal(err)
	}
	if gotUserID != 7 {
		t.Fatalf("foreign user id used: %d", gotUserID)
	}
	if strings.Contains(text, "[post:12]") || !strings.Contains(text, "[post:11]") {
		t.Fatalf("%s", text)
	}
	if len(sources) != 1 || sources[0].ID != "11" {
		t.Fatalf("%+v", sources)
	}
}

func TestGetMyLikesFiltersUnpublished(t *testing.T) {
	interaction := &fakeInteractionService{
		likes: func(_ context.Context, req *interactionservice.GetLikeListReq) (*interactionservice.GetLikeListResp, error) {
			if req.UserId != 3 || req.Page != 1 {
				t.Fatalf("%+v", req)
			}
			return &interactionservice.GetLikeListResp{PostIds: []int64{11, 12}, Total: 2}, nil
		},
	}
	content := &fakeContentService{postsByIDs: publishedBackfill(11)}
	registry, err := NewToolRegistry(Clients{Interaction: interaction, Content: content}, []string{ToolGetMyLikes})
	if err != nil {
		t.Fatal(err)
	}
	text, _, err := registry.Call(context.Background(), &Session{UserID: 3}, ToolGetMyLikes, "c1", `{"user_id":8}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "[post:12]") || !strings.Contains(text, "[post:11]") {
		t.Fatalf("%s", text)
	}
}

func TestGetMyFollowingDefaultsPage(t *testing.T) {
	var page, pageSize int32
	user := &fakeUserService{
		following: func(_ context.Context, req *userservice.GetFollowingReq) (*userservice.GetFollowingResp, error) {
			page, pageSize = req.Page, req.PageSize
			if req.UserId != 4 {
				t.Fatalf("user=%d", req.UserId)
			}
			return &userservice.GetFollowingResp{Users: []*userservice.UserInfo{
				{Id: 9, Username: "alice", Nickname: "Alice"},
			}, Total: 1}, nil
		},
	}
	registry, err := NewToolRegistry(Clients{User: user}, []string{ToolGetMyFollowing})
	if err != nil {
		t.Fatal(err)
	}
	text, sources, err := registry.Call(context.Background(), &Session{UserID: 4}, ToolGetMyFollowing, "c1", `{"page":0,"user_id":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if page != 1 || pageSize != 10 {
		t.Fatalf("page=%d size=%d", page, pageSize)
	}
	if len(sources) != 0 || !strings.Contains(text, "alice") && !strings.Contains(text, "Alice") {
		t.Fatalf("%s %+v", text, sources)
	}
}

func TestGetMyPostsUsesSessionUserAndFiltersDrafts(t *testing.T) {
	var gotUser int64
	content := &fakeContentService{
		userPosts: func(_ context.Context, req *contentservice.GetUserPostsReq) (*contentservice.GetUserPostsResp, error) {
			gotUser = req.UserId
			return &contentservice.GetUserPostsResp{Posts: []*contentservice.PostInfo{
				{Id: 11, Title: "pub", Content: "ok", Status: 1, Revision: 1},
				{Id: 12, Title: "draft", Content: "no", Status: 0, Revision: 1},
			}}, nil
		},
		postsByIDs: publishedBackfill(11),
	}
	registry, err := NewToolRegistry(Clients{Content: content}, []string{ToolGetMyPosts})
	if err != nil {
		t.Fatal(err)
	}
	text, _, err := registry.Call(context.Background(), &Session{UserID: 5}, ToolGetMyPosts, "c1", `{"user_id":99}`)
	if err != nil {
		t.Fatal(err)
	}
	if gotUser != 5 {
		t.Fatalf("user=%d", gotUser)
	}
	if strings.Contains(text, "[post:12]") || !strings.Contains(text, "[post:11]") {
		t.Fatalf("%s", text)
	}
}

func TestRestrictHidesUserStateOnV1Consent(t *testing.T) {
	registry, err := NewToolRegistry(Clients{}, append(Version1Tools(), ToolGetMyFavorites, ToolGetMyLikes, ToolGetMyFollowing, ToolGetMyPosts))
	if err != nil {
		t.Fatal(err)
	}
	filtered := RestrictToolsForConsent(registry, 1)
	if filtered.Has(ToolGetMyFavorites) || filtered.Has(ToolGetMyLikes) || filtered.Has(ToolGetMyFollowing) || filtered.Has(ToolGetMyPosts) {
		t.Fatal("v1 consent must hide userstate tools")
	}
	if !filtered.Has(ToolSearchPosts) {
		t.Fatal("v1 tools must remain")
	}
}

func TestGetMyFavoritesRequiresLogin(t *testing.T) {
	registry, err := NewToolRegistry(Clients{Interaction: &fakeInteractionService{}, Content: &fakeContentService{}}, []string{ToolGetMyFavorites})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = registry.Call(context.Background(), &Session{}, ToolGetMyFavorites, "c1", `{}`)
	if !errx.Is(err, errx.LoginRequired) {
		t.Fatalf("got %v", err)
	}
}
