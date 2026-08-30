package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/app/content/rpc/contentservice"

	"google.golang.org/grpc"
)

type failAfterEffectStore struct {
	store.Store
	failNext atomic.Bool
}

func (s *failAfterEffectStore) RunStep(ctx context.Context, fence store.LeaseFence, fn func(context.Context, store.Store) error) error {
	if s.failNext.CompareAndSwap(true, false) {
		return store.ErrLeaseLost
	}
	return s.Store.RunStep(ctx, fence, fn)
}

type idempotentUpdateContent struct {
	contentservice.ContentService
	mu         sync.Mutex
	store      *failAfterEffectStore
	applied    map[string]updateResult
	applyCount int
	requests   []updateRequest
}

type updateResult struct {
	status   int32
	revision int64
}

type updateRequest struct {
	idempotencyKey string
	revision       int64
}

func (c *idempotentUpdateContent) UpdatePost(
	_ context.Context,
	in *contentservice.UpdatePostReq,
	_ ...grpc.CallOption,
) (*contentservice.UpdatePostResp, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, updateRequest{idempotencyKey: in.IdempotencyKey, revision: in.ExpectedRevision})
	if replay, ok := c.applied[in.IdempotencyKey]; ok {
		return &contentservice.UpdatePostResp{Status: replay.status, Revision: replay.revision}, nil
	}
	c.applyCount++
	resp := updateResult{status: 1, revision: in.ExpectedRevision + 1}
	c.applied[in.IdempotencyKey] = resp
	// Simulate: Content committed, then the old worker lost its lease before
	// completing the Assistant journal/result step.
	c.store.failNext.Store(true)
	return &contentservice.UpdatePostResp{Status: resp.status, Revision: resp.revision}, nil
}

func TestPendingJournalRecoveryReplaysDownstreamIdempotencyOnlyOnce(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	wrapped := &failAfterEffectStore{Store: mem}
	content := &idempotentUpdateContent{store: wrapped, applied: map[string]updateResult{}}
	registry, err := tool.NewRegistry(tool.Clients{Content: content, Store: wrapped}, []string{tool.UpdatePost})
	if err != nil {
		t.Fatal(err)
	}
	session, err := mem.CreateSession(ctx, store.Session{UserID: 1, Status: store.SessionOpen})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := mem.InsertRun(ctx, store.Run{
		UserID: 1, SessionID: session.ID, RequestID: "request-1", Source: store.SourceUser,
		Status: store.StatusQueued, Phase: store.PhaseToolExecuting, ConsentVersion: 2, InputVersion: 1,
		CreatedAtMs: store.NowMs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := mem.Claim(ctx, "worker-one", store.NowMs(), 60_000)
	if err != nil || first == nil || first.ID != queued.ID {
		t.Fatalf("first claim: %+v err=%v", first, err)
	}
	engine := &Engine{Store: wrapped, Tools: registry}
	call := llm.ToolCall{
		ID: "call-1", Name: tool.UpdatePost,
		Arguments: `{"post_id":7,"title":"after","expected_revision":1}`,
		Prepared:  true,
	}
	err = engine.execTool(ctx, ctx, first, registry, call, nil)
	if err != store.ErrLeaseLost {
		t.Fatalf("first worker err=%v", err)
	}
	digest, err := canonical.DigestArgs(call.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := mem.GetJournal(ctx, first.UserID, first.RequestID, call.Name, digest)
	if err != nil || journal == nil || journal.Status != store.JournalPending {
		t.Fatalf("pending journal=%+v err=%v", journal, err)
	}

	mem.ExpireLease(first.ID, store.NowMs()-1)
	second, err := mem.Claim(ctx, "worker-two", store.NowMs(), 60_000)
	if err != nil || second == nil || second.LeaseGeneration <= first.LeaseGeneration {
		t.Fatalf("second claim: %+v err=%v", second, err)
	}
	if err := engine.execTool(ctx, ctx, second, registry, call, nil); err != nil {
		t.Fatal(err)
	}
	journal, err = mem.GetJournal(ctx, second.UserID, second.RequestID, call.Name, digest)
	if err != nil || journal == nil || journal.Status != store.JournalSuccess {
		t.Fatalf("completed journal=%+v err=%v", journal, err)
	}
	if content.applyCount != 1 || len(content.requests) != 2 {
		t.Fatalf("side effect applications=%d rpc calls=%d", content.applyCount, len(content.requests))
	}
	if content.requests[0].idempotencyKey == "" || content.requests[0].idempotencyKey != content.requests[1].idempotencyKey {
		t.Fatalf("unstable downstream idempotency keys: %q %q", content.requests[0].idempotencyKey, content.requests[1].idempotencyKey)
	}
	if content.requests[0].revision != 1 || content.requests[1].revision != 1 {
		t.Fatalf("revision was not frozen: %d %d", content.requests[0].revision, content.requests[1].revision)
	}
}
