package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"esx/app/assistant/internal/store"
	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"

	"google.golang.org/grpc"
)

type researchContent struct {
	contentservice.ContentService
	post     *contentservice.PostInfo
	comments []*contentservice.CommentInfo
}

func (c *researchContent) GetPost(context.Context, *contentservice.GetPostReq, ...grpc.CallOption) (*contentservice.GetPostResp, error) {
	return &contentservice.GetPostResp{Post: c.post}, nil
}
func (c *researchContent) GetCommentsByIds(context.Context, *contentservice.GetCommentsByIdsReq, ...grpc.CallOption) (*contentservice.GetCommentsByIdsResp, error) {
	return &contentservice.GetCommentsByIdsResp{Comments: c.comments}, nil
}

func researchFixture(t *testing.T) (Clients, *Session, store.Evidence) {
	t.Helper()
	ctx := context.Background()
	st := store.NewMemoryStore()
	content := &researchContent{post: &contentservice.PostInfo{Id: 9, Status: 1, Revision: 2, Title: "实际使用记录", Content: "维护成本低。需要每周检查。"}}
	ref := store.SourceRef{Handle: "h1", Kind: "post", AuthorityID: "9", Title: content.post.Title, Revision: 2, PayloadJSON: content.post.Content}
	raw, _ := json.Marshal(ref)
	if _, err := st.InsertSource(ctx, store.Source{RunID: 7, Handle: ref.Handle, Kind: ref.Kind, AuthorityID: ref.AuthorityID, Revision: ref.Revision, PayloadJSON: string(raw)}); err != nil {
		t.Fatal(err)
	}
	evidence := evidenceFor(7, "h1", "post", "维护成本低。", "")
	if err := st.PutEvidence(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	return Clients{Store: st, Content: content}, &Session{UserID: 1, RunID: 7, Source: store.SourceUser, ConsentVersion: 2, ClientProtocolVersion: 2}, evidence
}

func citedBlock(evidence store.Evidence) store.AnswerBlock {
	return store.AnswerBlock{Kind: "experience", Text: evidence.Text, Citations: []store.AnswerCitation{{Handle: evidence.Handle, EvidenceIDs: []string{evidence.ID}}}}
}

func TestResearchAnswerUsesRetrievedEvidence(t *testing.T) {
	clients, session, evidence := researchFixture(t)
	answer, err := BuildAnswer(context.Background(), clients, session, []store.AnswerBlock{citedBlock(evidence)})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Sources) != 1 || answer.Sources[0].Excerpts[0].Text != evidence.Text || !strings.Contains(AnswerText(answer), "[1](/post/9)") {
		t.Fatalf("answer=%+v", answer)
	}
	clients.Content.(*researchContent).post.Status = 0
	RevalidatePresentation(context.Background(), clients, session.UserID, answer)
	if answer.Sources[0].Available || len(answer.Sources[0].Excerpts) != 0 || answer.Sources[0].Title != "" {
		t.Fatalf("stale source exposed: %+v", answer.Sources[0])
	}
}

func TestResearchRejectsFakeAndStaleCitations(t *testing.T) {
	for _, kind := range []string{"fake_handle", "fake_evidence", "cross_run", "revision", "no_citations", "hidden_context"} {
		t.Run(kind, func(t *testing.T) {
			clients, session, evidence := researchFixture(t)
			block := citedBlock(evidence)
			switch kind {
			case "fake_handle":
				block.Citations[0].Handle = "forged"
			case "fake_evidence":
				block.Citations[0].EvidenceIDs = []string{"forged"}
			case "cross_run":
				session.RunID++
			case "revision":
				clients.Content.(*researchContent).post.Revision++
			case "no_citations":
				block.Citations = nil
			case "hidden_context":
				block.Text = "<untrusted-memory-context>private</untrusted-memory-context>"
			}
			if _, err := BuildAnswer(context.Background(), clients, session, []store.AnswerBlock{block}); err == nil {
				t.Fatal("invalid answer accepted")
			}
		})
	}
}

func TestReadSourcePublishesEvidenceIdsAndRejectsOtherRun(t *testing.T) {
	clients, session, _ := researchFixture(t)
	text, _, err := readSourceExecutor(clients)(context.Background(), session, "read", `{"handle":"h1","cursor":0}`)
	if err != nil || !strings.Contains(text, "ev_") || !strings.Contains(text, "每周检查") {
		t.Fatalf("result=%s err=%v", text, err)
	}
	session.RunID++
	if _, _, err := readSourceExecutor(clients)(context.Background(), session, "read", `{"handle":"h1"}`); !errx.Is(err, errx.NotFound) {
		t.Fatalf("cross run err=%v", err)
	}
}

func TestSourceURLSafety(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "file:///etc/passwd", "http://localhost/a", "http://127.0.0.1./a", "http://127.1/a", "http://2130706433/a", "http://10.0.0.1/a", "http://[::1]/a", "https://user:pass@example.com/a"} {
		if SafeSourceURL(raw) {
			t.Fatalf("unsafe URL accepted %s", raw)
		}
	}
	if !SafeSourceURL("https://example.com/article?q=topic#section") {
		t.Fatal("public URL rejected")
	}
}

func TestQuestionSchemaAndProtocolIsolation(t *testing.T) {
	clients, _, _ := researchFixture(t)
	registry, err := NewRegistry(clients, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ForClient(registry, 1).Has(AskQuestions) || !ForClient(registry, 2).Has(AskQuestions) {
		t.Fatal("protocol gate failed")
	}
	if ForSource(registry, store.SourceWatch, 2).Has(AskQuestions) || ForSource(registry, store.SourceMemoryReview, 2).Has(PublishAnswer) {
		t.Fatal("background tool boundary failed")
	}
	if ValidateQuestions([]store.Question{{ID: "q", Text: "问题", Selection: "single", Options: []store.QuestionOption{{ID: "a", Label: "一"}, {ID: "a", Label: "二"}}}}) == nil {
		t.Fatal("duplicate options accepted")
	}
}
