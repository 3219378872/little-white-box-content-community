package logic

import (
	"context"
	"testing"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/app/search/rpc/searchservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"

	"google.golang.org/grpc"
)

type createWatchUser struct {
	userservice.UserService
}

func (createWatchUser) GetUser(_ context.Context, req *userservice.GetUserReq, _ ...grpc.CallOption) (*userservice.GetUserResp, error) {
	if req.UserId == 8 {
		return &userservice.GetUserResp{User: &userservice.UserInfo{Id: 8}}, nil
	}
	return &userservice.GetUserResp{}, nil
}

type createWatchContent struct {
	contentservice.ContentService
}

func (createWatchContent) GetPost(_ context.Context, req *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
	if req.PostId == 11 {
		return &contentservice.GetPostResp{Post: &contentservice.PostInfo{Id: 11, Status: 1}}, nil
	}
	return &contentservice.GetPostResp{Post: &contentservice.PostInfo{Id: req.PostId, Status: 0}}, nil
}

func (createWatchContent) GetTags(context.Context, *contentservice.GetTagsReq, ...grpc.CallOption) (*contentservice.GetTagsResp, error) {
	return &contentservice.GetTagsResp{}, nil
}

type createWatchSearch struct {
	searchservice.SearchService
}

func (createWatchSearch) SearchTags(_ context.Context, req *searchservice.SearchTagsReq, _ ...grpc.CallOption) (*searchservice.SearchTagsResp, error) {
	if req.Keyword == "mhw" {
		return &searchservice.SearchTagsResp{Tags: []*searchservice.TagSearchResult{{Name: "mhw"}}}, nil
	}
	return &searchservice.SearchTagsResp{}, nil
}

func TestCreateWatchTaskLogicValidatesTargets(t *testing.T) {
	logic := NewCreateWatchTaskLogic(context.Background(), &svc.ServiceContext{
		Watch:          watch.NewMapStore(),
		UserService:    createWatchUser{},
		ContentService: createWatchContent{},
		SearchService:  createWatchSearch{},
	})
	if _, err := logic.CreateWatchTask(&pb.CreateWatchTaskReq{
		UserId: 2, ConditionType: watch.AuthorNewPost, TargetType: "author", TargetId: 8,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := logic.CreateWatchTask(&pb.CreateWatchTaskReq{
		UserId: 2, ConditionType: watch.AuthorNewPost, TargetType: "author", TargetId: 1,
	}); !errx.Is(err, errx.ParamError) {
		t.Fatalf("missing author: %v", err)
	}
	if _, err := logic.CreateWatchTask(&pb.CreateWatchTaskReq{
		UserId: 2, ConditionType: watch.PostRevised, TargetType: "post", TargetId: 99,
	}); !errx.Is(err, errx.ParamError) {
		t.Fatalf("unpublished: %v", err)
	}
	if _, err := logic.CreateWatchTask(&pb.CreateWatchTaskReq{
		UserId: 2, ConditionType: watch.TagNewPost, TargetType: "tag", TargetText: "nope",
	}); !errx.Is(err, errx.ParamError) {
		t.Fatalf("missing tag: %v", err)
	}
	if _, err := logic.CreateWatchTask(&pb.CreateWatchTaskReq{
		UserId: 2, ConditionType: watch.KeywordNewPost, TargetType: "keyword", TargetText: "怪猎",
	}); err != nil {
		t.Fatal(err)
	}
}
