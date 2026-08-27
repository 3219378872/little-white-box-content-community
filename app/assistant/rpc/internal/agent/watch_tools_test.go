package agent

import (
	"context"
	"testing"

	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"
)

func TestCreateWatchTaskValidatesTargets(t *testing.T) {
	store := watch.NewMapStore()
	user := &fakeUserService{
		getUser: func(_ context.Context, req *userservice.GetUserReq) (*userservice.GetUserResp, error) {
			if req.UserId == 8 {
				return &userservice.GetUserResp{User: &userservice.UserInfo{Id: 8}}, nil
			}
			return &userservice.GetUserResp{}, nil
		},
	}
	content := &fakeContentService{
		getPost: func(_ context.Context, req *contentservice.GetPostReq) (*contentservice.GetPostResp, error) {
			if req.PostId == 11 {
				return &contentservice.GetPostResp{Post: &contentservice.PostInfo{Id: 11, Status: 1}}, nil
			}
			if req.PostId == 12 {
				return &contentservice.GetPostResp{Post: &contentservice.PostInfo{Id: 12, Status: 0}}, nil
			}
			return &contentservice.GetPostResp{}, nil
		},
	}
	search := &fakeSearchService{
		tags: func(_ context.Context, req *searchservice.SearchTagsReq) (*searchservice.SearchTagsResp, error) {
			if req.Keyword == "mhw" {
				return &searchservice.SearchTagsResp{Tags: []*searchservice.TagSearchResult{{Name: "mhw", PostCount: 3}}}, nil
			}
			return &searchservice.SearchTagsResp{}, nil
		},
	}
	registry, err := NewToolRegistry(Clients{Watch: store, User: user, Content: content, Search: search}, []string{ToolCreateWatchTask})
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{UserID: 2}

	if _, _, err := registry.Call(context.Background(), session, ToolCreateWatchTask, "c1",
		`{"condition_type":"author_new_post","target_type":"author","target_id":8}`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.Call(context.Background(), session, ToolCreateWatchTask, "c2",
		`{"condition_type":"author_new_post","target_type":"author","target_id":99}`); !errx.Is(err, errx.ParamError) {
		t.Fatalf("missing author: %v", err)
	}
	if _, _, err := registry.Call(context.Background(), session, ToolCreateWatchTask, "c3",
		`{"condition_type":"post_revised","target_type":"post","target_id":12}`); !errx.Is(err, errx.ParamError) {
		t.Fatalf("draft post: %v", err)
	}
	if _, _, err := registry.Call(context.Background(), session, ToolCreateWatchTask, "c4",
		`{"condition_type":"discussion_spike","target_type":"post","target_id":11}`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.Call(context.Background(), session, ToolCreateWatchTask, "c5",
		`{"condition_type":"tag_new_post","target_type":"tag","target_text":"missing"}`); !errx.Is(err, errx.ParamError) {
		t.Fatalf("missing tag: %v", err)
	}
	if _, _, err := registry.Call(context.Background(), session, ToolCreateWatchTask, "c6",
		`{"condition_type":"keyword_new_post","target_type":"keyword","target_text":"怪猎"}`); err != nil {
		t.Fatal(err)
	}
}
