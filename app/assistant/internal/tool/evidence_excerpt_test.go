package tool

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"

	"google.golang.org/grpc"
)

func (c *researchContent) GetCommentList(context.Context, *contentservice.GetCommentListReq, ...grpc.CallOption) (*contentservice.GetCommentListResp, error) {
	return &contentservice.GetCommentListResp{Comments: c.comments}, nil
}

func TestRetrievedExcerptsRemainExactAndRejectChangedSources(t *testing.T) {
	for _, kind := range []string{"post", "comment"} {
		for _, length := range []int{10, 160, 360, 400} {
			t.Run(kind+"-"+strconv.Itoa(length), func(t *testing.T) {
				ctx := context.Background()
				st := store.NewMemoryStore()
				content := &researchContent{
					post:     &contentservice.PostInfo{Id: 9, Status: 1, Revision: 2, Title: "Source", Content: strings.Repeat("p", length)},
					comments: []*contentservice.CommentInfo{{Id: 11, PostId: 9, Status: 1, Content: strings.Repeat("c", length)}},
				}
				clients := Clients{Store: st, Content: content}
				session := &Session{UserID: 1, RunID: 7, Source: store.SourceUser, ConsentVersion: 2, ClientProtocolVersion: 2}
				registry, err := NewRegistry(clients, nil)
				if err != nil {
					t.Fatal(err)
				}
				name := GetPost
				if kind == "comment" {
					name = GetPostComments
				}
				if _, _, err := registry.Call(ctx, session, name, "retrieve", `{"post_id":9}`); err != nil {
					t.Fatal(err)
				}
				sources, err := st.ListSources(ctx, session.RunID)
				if err != nil || len(sources) != 1 {
					t.Fatalf("sources=%+v err=%v", sources, err)
				}
				items, err := st.ListEvidence(ctx, session.RunID, sources[0].Handle)
				if err != nil {
					t.Fatal(err)
				}
				for _, evidence := range items {
					if evidence.Kind != kind {
						continue
					}
					original := content.post.Content
					if kind == "comment" {
						original = content.comments[0].Content
					}
					if !strings.Contains(original, evidence.Text) {
						t.Fatalf("non-original evidence=%q", evidence.Text)
					}
					blocks := []store.AnswerBlock{citedBlock(evidence)}
					if _, err := BuildAnswer(ctx, clients, session, blocks); err != nil {
						t.Fatalf("valid evidence rejected: %v", err)
					}
					if kind == "comment" {
						content.comments[0].Content = "changed"
					} else {
						content.post.Content = "changed"
					}
					if _, err := BuildAnswer(ctx, clients, session, blocks); !errx.Is(err, errx.ContentVersionConflict) {
						t.Fatalf("changed source error=%v", err)
					}
					return
				}
				t.Fatalf("missing %s evidence", kind)
			})
		}
	}
}

func TestEvidenceExcerptCountsUnicodeWithoutAddingText(t *testing.T) {
	original := strings.Repeat("\u8bed", 400)
	excerpt := excerptRunes(original, maxEvidenceSnippetRunes)
	if len([]rune(excerpt)) != maxEvidenceSnippetRunes || !strings.HasPrefix(original, excerpt) {
		t.Fatalf("excerpt does not preserve Unicode prefix: %q", excerpt)
	}
}
