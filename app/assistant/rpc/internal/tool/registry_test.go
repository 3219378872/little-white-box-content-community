package tool

import (
	"context"
	"testing"

	"errx"
	"esx/app/search/rpc/searchservice"

	"google.golang.org/grpc"
)

type fakeSearchService struct {
	searchservice.SearchService
	search func(context.Context, *searchservice.SearchReq) (*searchservice.SearchResp, error)
}

func (f fakeSearchService) Search(ctx context.Context, request *searchservice.SearchReq, _ ...grpc.CallOption) (*searchservice.SearchResp, error) {
	return f.search(ctx, request)
}

func TestRegistryRejectsUnknownConfiguredTool(t *testing.T) {
	t.Parallel()
	if _, err := NewRegistry([]string{"database"}, Clients{}, 5); err == nil {
		t.Fatal("expected unknown tool configuration to fail")
	}
}

func TestRegistryEnforcesAllowlistBeforeCallingTool(t *testing.T) {
	t.Parallel()
	called := false
	registry, err := NewRegistry([]string{"user"}, Clients{Search: fakeSearchService{search: func(context.Context, *searchservice.SearchReq) (*searchservice.SearchResp, error) {
		called = true
		return &searchservice.SearchResp{}, nil
	}}}, 5)
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Execute(context.Background(), Search, Request{Message: "go"})
	if !errx.Is(err, errx.PermissionDenied) {
		t.Fatalf("error=%v want permission denied", err)
	}
	if called {
		t.Fatal("disallowed tool was called")
	}
}

func TestRegistrySearchUsesOriginalContextAndReturnsSources(t *testing.T) {
	t.Parallel()
	type contextKey string
	const marker contextKey = "marker"
	ctx := context.WithValue(context.Background(), marker, "preserved")
	registry, err := NewRegistry([]string{"search"}, Clients{Search: fakeSearchService{search: func(callCtx context.Context, request *searchservice.SearchReq) (*searchservice.SearchResp, error) {
		if callCtx.Value(marker) != "preserved" {
			t.Fatal("context value was not propagated")
		}
		if request.Keyword != "go" || request.Page != 1 || request.PageSize != 2 {
			t.Fatalf("unexpected search request: %+v", request)
		}
		return &searchservice.SearchResp{Posts: []*searchservice.PostSearchResult{{Id: 11, Title: "Go post"}}}, nil
	}}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Execute(ctx, Search, Request{Message: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text == "" || len(result.Sources) != 1 || result.Sources[0].ID != "11" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
