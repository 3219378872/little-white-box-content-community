package logic

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"esx/app/review/internal/policy"
	"esx/app/review/internal/store"
	"esx/app/review/rpc/internal/svc"
	"esx/pkg/errx"
	"esx/pkg/event"
)

var (
	testNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	errBoom = errors.New("boom")
)

// fakeStore 是 svc.Store 的内存实现。reviewers 与 tasks 按 ID 查找，*Err 字段注入对应方法的失败。
type fakeStore struct {
	reviewers map[int64]*store.Reviewer
	tasks     map[int64]*store.Task
	snapshots map[string]string
	stages    map[int64][]store.Stage
	decisions map[int64]*store.Decision
	claimed   *store.Task
	buckets   []store.QueueBucket
	seeds     []store.Seed
	seed      *store.Seed

	reviewerErr, claimErr, renewErr, releaseErr, getTaskErr, snapshotErr, stagesErr,
	decisionErr, submitErr, queueErr, seedsErr, transitionErr error

	submitted    store.DecisionInput
	seedStatus   string
	transitionTo string

	activatedAt, seedChanges, seedChangedAt int64
	activated                               bool
	activationErr, seedGenErr               error
}

func (f *fakeStore) SeedGeneration(context.Context) (int64, int64, error) {
	return f.seedChanges, f.seedChangedAt, f.seedGenErr
}

func (f *fakeStore) PolicyActivatedAt(_ context.Context, version string) (int64, bool, error) {
	return f.activatedAt, f.activated && version != "", f.activationErr
}

func (f *fakeStore) Reviewer(_ context.Context, userID int64) (*store.Reviewer, error) {
	if f.reviewerErr != nil {
		return nil, f.reviewerErr
	}
	r, ok := f.reviewers[userID]
	if !ok {
		return nil, store.ErrReviewerUnknown
	}
	return r, nil
}

func (f *fakeStore) ClaimHuman(context.Context, *store.Reviewer, string, time.Time) (*store.Task, error) {
	return f.claimed, f.claimErr
}

func (f *fakeStore) Renew(_ context.Context, _, taskID, _ int64, _ time.Time) (*store.Task, error) {
	if f.renewErr != nil {
		return nil, f.renewErr
	}
	return f.tasks[taskID], nil
}

func (f *fakeStore) Release(context.Context, int64, int64, int64, time.Time) error {
	return f.releaseErr
}

func (f *fakeStore) GetTask(_ context.Context, id int64) (*store.Task, error) {
	if f.getTaskErr != nil {
		return nil, f.getTaskErr
	}
	task, ok := f.tasks[id]
	if !ok {
		return nil, store.ErrTaskNotFound
	}
	return task, nil
}

func (f *fakeStore) TaskSnapshotContent(ctx context.Context, taskID int64) (*store.Task, string, error) {
	task, err := f.GetTask(ctx, taskID)
	if err != nil {
		return nil, "", err
	}
	return task, f.snapshots[task.SnapshotHash], nil
}

func (f *fakeStore) Snapshot(_ context.Context, hash string) (string, error) {
	return f.snapshots[hash], f.snapshotErr
}

func (f *fakeStore) Stages(_ context.Context, taskID int64) ([]store.Stage, error) {
	return f.stages[taskID], f.stagesErr
}

func (f *fakeStore) DecisionByTask(_ context.Context, taskID int64) (*store.Decision, error) {
	return f.decisions[taskID], f.decisionErr
}

func (f *fakeStore) SubmitHuman(_ context.Context, _, _, _ int64, _ string, in store.DecisionInput, _ time.Time) (*store.Decision, error) {
	f.submitted = in
	if f.submitErr != nil {
		return nil, f.submitErr
	}
	return &store.Decision{
		ID: 900, Verdict: in.Verdict, PolicyVersion: in.PolicyVersion, Source: in.Source, PolicyCodesJSON: mustJSON(in.PolicyCodes),
	}, nil
}

func (f *fakeStore) QueueSummary(context.Context, *store.Reviewer, time.Time) ([]store.QueueBucket, error) {
	return f.buckets, f.queueErr
}

func (f *fakeStore) ListSeeds(_ context.Context, status string, _ int) ([]store.Seed, error) {
	f.seedStatus = status
	return f.seeds, f.seedsErr
}

func (f *fakeStore) TransitionSeed(_ context.Context, _, _ int64, to string, _ time.Time) (*store.Seed, error) {
	f.transitionTo = to
	if f.transitionErr != nil {
		return nil, f.transitionErr
	}
	seed := *f.seed
	seed.Status = to
	return &seed, nil
}

type fakeIngester struct {
	result store.IngestResult
	err    error
}

func (f *fakeIngester) Ingest(context.Context, event.ReviewSubmittedEvent) (store.IngestResult, error) {
	return f.result, f.err
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

const (
	reviewerID = int64(1)
	adminID    = int64(2)
	qualID     = int64(3)
	inactiveID = int64(4)
)

// newFixture 准备：普通审核员（US/en）、政策管理员、资质审核员与已停用审核员；
// 任务 100 为初审，101 为质检（源任务 100），102 属于 DE 市场。
func newFixture() (*svc.ServiceContext, *fakeStore) {
	snapshot := mustJSON(map[string]any{"snapshot": event.ReviewSnapshot{
		Texts: map[string]string{"title": "Buy now", "body": "Cheap"}, Market: "US", Language: "en", SubmitterID: 9,
		Media:          []event.ReviewMedia{{MediaID: 50, SHA256: "a"}},
		Qualifications: []event.ReviewQualification{{DocumentMediaID: 60}},
	}})
	fs := &fakeStore{
		reviewers: map[int64]*store.Reviewer{
			reviewerID: {UserID: reviewerID, RolesCSV: "reviewer,qa", MarketsCSV: "US", LanguageCSV: "en", Active: true},
			adminID:    {UserID: adminID, RolesCSV: "policy_admin", MarketsCSV: "US", LanguageCSV: "en", Active: true},
			qualID:     {UserID: qualID, RolesCSV: "qualification_reviewer", MarketsCSV: "US", LanguageCSV: "en", Active: true},
			inactiveID: {UserID: inactiveID, RolesCSV: "reviewer", MarketsCSV: "US", LanguageCSV: "en", Active: false},
		},
		tasks: map[int64]*store.Task{
			100: {ID: 100, Purpose: event.ReviewPurposeInitial, RequiredRole: store.RoleReviewer, Market: "US", Language: "en", SnapshotHash: "h"},
			101: {ID: 101, Purpose: event.ReviewPurposeQA, RequiredRole: store.RoleQA, Market: "US", Language: "en", SnapshotHash: "h", SourceTaskID: 100, DecisionID: 7},
			102: {ID: 102, Purpose: event.ReviewPurposeInitial, RequiredRole: store.RoleReviewer, Market: "DE", Language: "de", SnapshotHash: "h"},
		},
		snapshots: map[string]string{"h": snapshot},
		stages:    map[int64][]store.Stage{100: {{Stage: "rules", Outcome: "pass"}}, 101: {{Stage: "qa", Outcome: "pending"}}},
		decisions: map[int64]*store.Decision{
			100: {ID: 6, Verdict: event.ReviewVerdictApprove, Source: event.ReviewSourceMachine, PolicyCodesJSON: "[]"},
			101: {ID: 7, Verdict: event.ReviewVerdictReject, Source: event.ReviewSourceQA, PolicyCodesJSON: `["CONTENT.IP"]`},
		},
		seed: &store.Seed{ID: 30, IssueCode: "CONTENT.IP", Market: "US", Status: store.SeedCandidate},
	}
	return &svc.ServiceContext{
		Store: fs, Policy: &policy.Policy{Version: "demo-v1"}, Ingester: &fakeIngester{},
		Clock: func() time.Time { return testNow },
	}, fs
}

func requireCode(t *testing.T, err error, code int) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, code, errx.GetCode(err), "err=%v", err)
}
