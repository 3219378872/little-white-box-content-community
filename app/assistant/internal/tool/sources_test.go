package tool

import (
	"context"
	"testing"

	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"

	"google.golang.org/grpc"
)

type sourceContent struct {
	contentservice.ContentService
	posts []*contentservice.PostInfo
}

func (s *sourceContent) GetPostsByIds(_ context.Context, _ *contentservice.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error) {
	return &contentservice.GetPostsByIdsResp{Posts: s.posts}, nil
}

func TestSourceHandleRunBindingAndPresentSources(t *testing.T) {
	mem := store.NewMemoryStore()
	content := &sourceContent{posts: []*contentservice.PostInfo{{Id: 9, Status: 1, Revision: 1, Title: "仍可见", Content: "当前正文"}}}
	reg, err := NewRegistry(Clients{Store: mem, Content: content}, []string{PresentSources, SearchPosts})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sess := &Session{UserID: 1, RunID: 11, RequestID: "r"}
	_, err = mem.InsertSource(ctx, store.Source{RunID: 11, Handle: "src_ok", Kind: "post", AuthorityID: "9", Revision: 1, CreatedAtMs: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = mem.InsertSource(ctx, store.Source{RunID: 99, Handle: "src_other", Kind: "post", AuthorityID: "8", CreatedAtMs: 1})
	if err != nil {
		t.Fatal(err)
	}
	text, cards, err := reg.Call(ctx, sess, PresentSources, "c1", `{"handles":["src_ok","src_other","forged"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].Handle != "src_ok" {
		t.Fatalf("cards=%+v text=%s", cards, text)
	}
	quoted := `"{\"handles\":[\"src_ok\"]}"`
	text, cards, err = reg.Call(ctx, sess, PresentSources, "c2", quoted)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].Handle != "src_ok" {
		t.Fatalf("quoted cards=%+v text=%s", cards, text)
	}
	content.posts[0].Revision = 2
	text, cards, err = reg.Call(ctx, sess, PresentSources, "c3", `{"handles":["src_ok"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 0 || text == "" {
		t.Fatalf("revision-changed cards=%+v text=%s", cards, text)
	}
}
