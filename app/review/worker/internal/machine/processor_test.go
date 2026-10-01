package machine

import (
	"context"
	"testing"
	"time"

	"esx/app/review/internal/cascade"
	"esx/app/review/internal/policy"
	"esx/app/review/internal/snapshot"
	"esx/app/review/internal/store"
	"esx/pkg/event"

	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	content   string
	stages    []store.Stage
	decisions []store.DecisionInput
	escalated []string
	decideErr error
}

func (f *fakeStore) Snapshot(context.Context, string) (string, error) { return f.content, nil }

func (f *fakeStore) RecordStage(_ context.Context, s store.Stage, _ time.Time) error {
	f.stages = append(f.stages, s)
	return nil
}

func (f *fakeStore) DecideMachine(_ context.Context, task *store.Task, in store.DecisionInput, _ int64, _ time.Time) (*store.Decision, error) {
	if f.decideErr != nil {
		return nil, f.decideErr
	}
	f.decisions = append(f.decisions, in)
	return &store.Decision{TaskID: task.ID, Verdict: in.Verdict, Source: in.Source}, nil
}

func (f *fakeStore) Escalate(_ context.Context, _ *store.Task, reason string, _ int, _ time.Time) error {
	f.escalated = append(f.escalated, reason)
	return nil
}

type lowRouter struct{}

func (lowRouter) Route(context.Context, cascade.RouteInput) (cascade.RouteResult, error) {
	return cascade.RouteResult{Version: "seedbank@test", Covered: true, Similarity: map[string]float64{}}, nil
}
func (lowRouter) Version() string { return "seedbank@test" }

func frozenContent(t *testing.T, industry string) string {
	t.Helper()
	frozen, err := snapshot.Freeze(event.ReviewBizAdCreative, event.ReviewSnapshot{
		Texts: map[string]string{"title": "Coffee"}, Market: "US", Language: "en", Industry: industry,
		SubmitterID: 1, LandingURL: "https://coffee.shop.com",
	})
	require.NoError(t, err)
	return string(frozen.Content)
}

func newProcessor(t *testing.T, fs *fakeStore, sampled bool) *Processor {
	t.Helper()
	active, err := policy.Load("ads-2026-10-01")
	require.NoError(t, err)
	return &Processor{
		Store: fs, Cascade: &cascade.Cascade{Router: lowRouter{}}, Active: active,
		Sample: func(rate float64) bool { require.GreaterOrEqual(t, rate, 0.05); return sampled },
	}
}

func task(seq int) *store.Task {
	return &store.Task{ID: 9, BizType: event.ReviewBizAdCreative, Purpose: event.ReviewPurposeInitial, SnapshotHash: "h", SubmissionSeq: seq}
}

func TestAutoApproveCreatesSampledQA(t *testing.T) {
	fs := &fakeStore{content: frozenContent(t, "GENERAL")}
	require.NoError(t, newProcessor(t, fs, true).Process(context.Background(), task(4)))
	require.Len(t, fs.decisions, 1)
	require.Equal(t, event.ReviewVerdictApprove, fs.decisions[0].Verdict)
	require.Equal(t, event.ReviewSourceMachine, fs.decisions[0].Source)
	require.True(t, fs.decisions[0].CreateQA)
	require.NotEmpty(t, fs.stages)
}

func TestProtectedSubmissionEscalates(t *testing.T) {
	fs := &fakeStore{content: frozenContent(t, "GENERAL")}
	require.NoError(t, newProcessor(t, fs, true).Process(context.Background(), task(1)))
	require.Empty(t, fs.decisions)
	require.Equal(t, []string{cascade.ReasonProtection}, fs.escalated)
}

func TestHardRuleRejectsWithoutQA(t *testing.T) {
	fs := &fakeStore{content: frozenContent(t, "ALCOHOL")}
	require.NoError(t, newProcessor(t, fs, true).Process(context.Background(), task(1)))
	require.Len(t, fs.decisions, 1)
	require.Equal(t, []string{"INDUSTRY.ALCOHOL"}, fs.decisions[0].PolicyCodes)
	require.False(t, fs.decisions[0].CreateQA)
}

// RVW-016：影子版本只写阶段记录，不改变结论。
func TestShadowRunOnlyRecordsStages(t *testing.T) {
	fs := &fakeStore{content: frozenContent(t, "GENERAL")}
	p := newProcessor(t, fs, false)
	p.Shadow = p.Active
	require.NoError(t, p.Process(context.Background(), task(4)))
	require.Len(t, fs.decisions, 1)
	shadow := 0
	for _, s := range fs.stages {
		if s.Shadow {
			shadow++
		}
	}
	require.Positive(t, shadow)
	require.Equal(t, len(fs.stages)-shadow, shadow)
}

func TestFencedDecisionIsDropped(t *testing.T) {
	fs := &fakeStore{content: frozenContent(t, "GENERAL"), decideErr: store.ErrTaskSuperseded}
	require.NoError(t, newProcessor(t, fs, false).Process(context.Background(), task(4)))
}

func TestCorruptSnapshotEscalates(t *testing.T) {
	fs := &fakeStore{content: "{"}
	require.NoError(t, newProcessor(t, fs, false).Process(context.Background(), task(4)))
	require.Equal(t, []string{"snapshot-invalid"}, fs.escalated)
}
