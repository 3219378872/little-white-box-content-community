package tool

import (
	"context"
	"sort"
	"testing"

	"github.com/cloudwego/kitex/client/callopt"

	"esx/app/search/rpc/searchservice"
	"esx/pkg/errx"
)

type emptyPostSearch struct {
	searchservice.SearchService
	calls int
}

func (s *emptyPostSearch) SearchPosts(context.Context, *searchservice.SearchPostsReq, ...callopt.Option) (*searchservice.SearchPostsResp, error) {
	s.calls++
	return &searchservice.SearchPostsResp{}, nil
}

func TestSearchPostsSchemaOnlyAdvertisesHonoredArguments(t *testing.T) {
	search := &emptyPostSearch{}
	reg, err := NewRegistry(Clients{Search: search, Content: &sourceContent{}}, []string{SearchPosts})
	if err != nil {
		t.Fatal(err)
	}
	var properties []string
	for _, def := range reg.Definitions() {
		if def.Name != SearchPosts {
			continue
		}
		for name := range def.Parameters["properties"].(map[string]any) {
			properties = append(properties, name)
		}
	}
	sort.Strings(properties)
	want := []string{"keyword", "page", "page_size", "sort_by", "tags"}
	if len(properties) != len(want) {
		t.Fatalf("search_posts properties=%v want %v", properties, want)
	}
	for i := range want {
		if properties[i] != want[i] {
			t.Fatalf("search_posts properties=%v want %v", properties, want)
		}
	}

	ctx := context.Background()
	sess := &Session{UserID: 1, RunID: 1, RequestID: "r"}
	for _, args := range []string{`{"keyword":"相机","include_comments":true}`, `{"keyword":"相机","seed_post_id":9}`} {
		if _, _, err := reg.Call(ctx, sess, SearchPosts, "c1", args); !errx.Is(err, errx.ParamError) {
			t.Fatalf("args %s: err=%v, want ParamError", args, err)
		}
	}
	if search.calls != 0 {
		t.Fatalf("rejected arguments reached search %d times", search.calls)
	}
}
