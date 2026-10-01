package logic

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"esx/app/review/internal/intake"
	"esx/app/review/internal/store"
	pb "esx/kitex_gen/review"
	"esx/pkg/errx"
	"esx/pkg/event"
	"esx/pkg/idempotencyx"
)

func TestReviewerGate(t *testing.T) {
	svcCtx, fs := newFixture()
	b := newBase(context.Background(), svcCtx)

	_, err := b.reviewer(0)
	requireCode(t, err, errx.LoginRequired)
	_, err = b.reviewer(99)
	requireCode(t, err, errx.ReviewRoleRequired)
	_, err = b.reviewer(inactiveID)
	requireCode(t, err, errx.ReviewRoleRequired)
	_, err = b.reviewer(adminID, store.RoleReviewer)
	requireCode(t, err, errx.ReviewRoleRequired)
	r, err := b.reviewer(adminID)
	require.NoError(t, err)
	require.Equal(t, adminID, r.UserID)

	fs.reviewerErr = errBoom
	_, err = b.reviewer(reviewerID)
	requireCode(t, err, errx.SystemError)
}

func TestMapStoreError(t *testing.T) {
	_, invalid := intake.Decode([]byte("{"))
	cases := map[error]int{
		store.ErrLeaseLost:                  errx.ReviewLeaseLost,
		store.ErrTaskSuperseded:             errx.ReviewTaskSuperseded,
		store.ErrTaskDecided:                errx.ReviewTaskDecided,
		store.ErrTaskNotFound:               errx.NotFound,
		store.ErrSeedNotFound:               errx.NotFound,
		store.ErrSameActor:                  errx.PermissionDenied,
		store.ErrSeedTransition:             errx.ParamError,
		idempotencyx.ErrIdempotencyConflict: errx.IdempotencyConflict,
		invalid:                             errx.ParamError,
		errBoom:                             errx.SystemError,
	}
	b := newBase(context.Background(), nil)
	for in, code := range cases {
		requireCode(t, b.mapStoreError(in, "test"), code)
	}
	require.NoError(t, b.mapStoreError(nil, "test"))
}

func TestClaimTask(t *testing.T) {
	ctx := context.Background()
	svcCtx, fs := newFixture()

	_, err := NewClaimTaskLogic(ctx, svcCtx).ClaimTask(&pb.ClaimTaskReq{UserId: reviewerID, Purpose: "bogus"})
	requireCode(t, err, errx.ParamError)

	resp, err := NewClaimTaskLogic(ctx, svcCtx).ClaimTask(&pb.ClaimTaskReq{UserId: reviewerID})
	require.NoError(t, err)
	require.False(t, resp.Found, "empty queue")

	task := *fs.tasks[101]
	task.Attempts = store.MaxHumanAttempts + 1
	fs.claimed = &task
	resp, err = NewClaimTaskLogic(ctx, svcCtx).ClaimTask(&pb.ClaimTaskReq{UserId: reviewerID, Purpose: event.ReviewPurposeQA})
	require.NoError(t, err)
	require.True(t, resp.Found)
	view := resp.Task
	require.Equal(t, int64(101), view.TaskId)
	require.NotEmpty(t, view.SnapshotJson)
	require.Len(t, view.Stages, 2, "QA shows its own stages followed by the original task's")
	require.Equal(t, event.ReviewVerdictApprove, view.OriginalDecision.Verdict)
	require.Equal(t, []string{"CONTENT.IP"}, view.Decision.PolicyCodes)

	fs.stagesErr = errBoom
	_, err = NewClaimTaskLogic(ctx, svcCtx).ClaimTask(&pb.ClaimTaskReq{UserId: reviewerID})
	requireCode(t, err, errx.SystemError)

	fs.claimErr = store.ErrLeaseLost
	_, err = NewClaimTaskLogic(ctx, svcCtx).ClaimTask(&pb.ClaimTaskReq{UserId: reviewerID})
	requireCode(t, err, errx.ReviewLeaseLost)

	_, err = NewClaimTaskLogic(ctx, svcCtx).ClaimTask(&pb.ClaimTaskReq{UserId: adminID})
	requireCode(t, err, errx.ReviewRoleRequired)
}

func TestTaskViewEvidenceFailures(t *testing.T) {
	cases := map[string]func(*fakeStore){
		"snapshot":  func(fs *fakeStore) { fs.snapshotErr = errBoom },
		"stages":    func(fs *fakeStore) { fs.stagesErr = errBoom },
		"decisions": func(fs *fakeStore) { fs.decisionErr = errBoom },
	}
	for name, breakStore := range cases {
		t.Run(name, func(t *testing.T) {
			svcCtx, fs := newFixture()
			breakStore(fs)
			_, err := newBase(context.Background(), svcCtx).taskView(fs.tasks[101], true)
			require.ErrorIs(t, err, errBoom)
		})
	}

	svcCtx, fs := newFixture()
	view, err := newBase(context.Background(), svcCtx).taskView(fs.tasks[100], false)
	require.NoError(t, err)
	require.Empty(t, view.SnapshotJson)
	require.Nil(t, decisionView(nil))
}

func TestRenewAndReleaseTask(t *testing.T) {
	ctx := context.Background()
	svcCtx, fs := newFixture()

	resp, err := NewRenewTaskLogic(ctx, svcCtx).RenewTask(&pb.LeaseReq{UserId: reviewerID, TaskId: 100, LeaseGeneration: 2})
	require.NoError(t, err)
	require.Equal(t, int64(100), resp.Task.TaskId)
	require.Empty(t, resp.Task.Stages, "renew returns the lease state without evidence")

	_, err = NewReleaseTaskLogic(ctx, svcCtx).ReleaseTask(&pb.LeaseReq{UserId: reviewerID, TaskId: 100})
	require.NoError(t, err)

	fs.renewErr, fs.releaseErr = store.ErrLeaseLost, store.ErrTaskDecided
	_, err = NewRenewTaskLogic(ctx, svcCtx).RenewTask(&pb.LeaseReq{UserId: reviewerID, TaskId: 100})
	requireCode(t, err, errx.ReviewLeaseLost)
	_, err = NewReleaseTaskLogic(ctx, svcCtx).ReleaseTask(&pb.LeaseReq{UserId: reviewerID, TaskId: 100})
	requireCode(t, err, errx.ReviewTaskDecided)

	for _, call := range []func() error{
		func() error {
			_, err := NewRenewTaskLogic(ctx, svcCtx).RenewTask(&pb.LeaseReq{UserId: adminID})
			return err
		},
		func() error {
			_, err := NewReleaseTaskLogic(ctx, svcCtx).ReleaseTask(&pb.LeaseReq{UserId: adminID})
			return err
		},
	} {
		requireCode(t, call(), errx.ReviewRoleRequired)
	}
}

func TestGetTaskHidesTasksOutsideTheReviewersScope(t *testing.T) {
	ctx := context.Background()
	svcCtx, fs := newFixture()

	resp, err := NewGetTaskLogic(ctx, svcCtx).GetTask(&pb.GetTaskReq{UserId: reviewerID, TaskId: 100})
	require.NoError(t, err)
	require.True(t, resp.Found)

	_, err = NewGetTaskLogic(ctx, svcCtx).GetTask(&pb.GetTaskReq{UserId: reviewerID, TaskId: 102})
	requireCode(t, err, errx.NotFound)
	_, err = NewGetTaskLogic(ctx, svcCtx).GetTask(&pb.GetTaskReq{UserId: qualID, TaskId: 100})
	requireCode(t, err, errx.NotFound)
	_, err = NewGetTaskLogic(ctx, svcCtx).GetTask(&pb.GetTaskReq{UserId: reviewerID, TaskId: 404})
	requireCode(t, err, errx.NotFound)

	fs.snapshotErr = errBoom
	_, err = NewGetTaskLogic(ctx, svcCtx).GetTask(&pb.GetTaskReq{UserId: reviewerID, TaskId: 100})
	requireCode(t, err, errx.SystemError)

	_, err = NewGetTaskLogic(ctx, svcCtx).GetTask(&pb.GetTaskReq{UserId: 0})
	requireCode(t, err, errx.LoginRequired)
}

func TestValidateVerdict(t *testing.T) {
	codes, err := validateVerdict(event.ReviewVerdictReject, []string{" CONTENT.DECEPTIVE ", "CONTENT.DECEPTIVE"})
	require.NoError(t, err)
	require.Equal(t, []string{"CONTENT.DECEPTIVE"}, codes)

	codes, err = validateVerdict(event.ReviewVerdictApprove, nil)
	require.NoError(t, err)
	require.Empty(t, codes)

	for name, tc := range map[string]struct {
		verdict string
		codes   []string
	}{
		"unknown code":         {event.ReviewVerdictReject, []string{"NOPE"}},
		"approve with codes":   {event.ReviewVerdictApprove, []string{"CONTENT.DECEPTIVE"}},
		"reject without codes": {event.ReviewVerdictReject, nil},
		"unknown verdict":      {"maybe", nil},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := validateVerdict(tc.verdict, tc.codes)
			requireCode(t, err, errx.ParamError)
		})
	}
}

func TestSubmitDecision(t *testing.T) {
	ctx := context.Background()
	reject := func(taskID int64) *pb.SubmitDecisionReq {
		return &pb.SubmitDecisionReq{
			UserId: reviewerID, TaskId: taskID, Verdict: event.ReviewVerdictReject,
			PolicyCodes: []string{"CONTENT.DECEPTIVE"}, NominateSeed: true,
		}
	}

	t.Run("reject with seed nomination", func(t *testing.T) {
		svcCtx, fs := newFixture()
		resp, err := NewSubmitDecisionLogic(ctx, svcCtx).SubmitDecision(reject(100))
		require.NoError(t, err)
		require.Equal(t, int64(900), resp.DecisionId)
		require.Equal(t, "demo-v1", resp.PolicyVersion)
		require.Equal(t, []string{"CONTENT.DECEPTIVE"}, resp.PolicyCodes)
		require.Equal(t, event.ReviewSourceHuman, fs.submitted.Source)
		require.Contains(t, fs.submitted.SeedText, "Buy now")
	})

	t.Run("QA disagreement and appeal overturn are observed", func(t *testing.T) {
		svcCtx, fs := newFixture()
		_, err := NewSubmitDecisionLogic(ctx, svcCtx).SubmitDecision(reject(101))
		require.NoError(t, err)
		require.Equal(t, event.ReviewSourceQA, fs.submitted.Source)

		fs.tasks[101].Purpose = event.ReviewPurposeAppeal
		_, err = NewSubmitDecisionLogic(ctx, svcCtx).SubmitDecision(&pb.SubmitDecisionReq{
			UserId: reviewerID, TaskId: 101, Verdict: event.ReviewVerdictApprove,
		})
		require.NoError(t, err)
	})

	invalid := map[string]func(*pb.SubmitDecisionReq){
		"verdict":     func(r *pb.SubmitDecisionReq) { r.Verdict = "maybe" },
		"long note":   func(r *pb.SubmitDecisionReq) { r.Note = strings.Repeat("n", 501) },
		"idempotency": func(r *pb.SubmitDecisionReq) { r.IdempotencyKey = strings.Repeat("k", 129) },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			svcCtx, _ := newFixture()
			req := reject(100)
			mutate(req)
			_, err := NewSubmitDecisionLogic(ctx, svcCtx).SubmitDecision(req)
			requireCode(t, err, errx.ParamError)
		})
	}

	failures := map[string]struct {
		req   *pb.SubmitDecisionReq
		setup func(*fakeStore)
		code  int
	}{
		"not a reviewer":   {req: &pb.SubmitDecisionReq{UserId: adminID}, code: errx.ReviewRoleRequired},
		"unknown task":     {req: reject(404), code: errx.NotFound},
		"out of scope":     {req: reject(102), code: errx.NotFound},
		"snapshot":         {req: reject(100), setup: func(fs *fakeStore) { fs.snapshotErr = errBoom }, code: errx.SystemError},
		"corrupt snapshot": {req: reject(100), setup: func(fs *fakeStore) { fs.snapshots["h"] = "{" }, code: errx.SystemError},
		"lease lost":       {req: reject(100), setup: func(fs *fakeStore) { fs.submitErr = store.ErrLeaseLost }, code: errx.ReviewLeaseLost},
	}
	for name, tc := range failures {
		t.Run(name, func(t *testing.T) {
			svcCtx, fs := newFixture()
			if tc.setup != nil {
				tc.setup(fs)
			}
			_, err := NewSubmitDecisionLogic(ctx, svcCtx).SubmitDecision(tc.req)
			requireCode(t, err, tc.code)
		})
	}
}

func TestAuthorizeEvidenceMedia(t *testing.T) {
	ctx := context.Background()
	svcCtx, fs := newFixture()
	allowed := func(userID, taskID, mediaID int64) bool {
		t.Helper()
		resp, err := NewAuthorizeEvidenceMediaLogic(ctx, svcCtx).AuthorizeEvidenceMedia(&pb.AuthorizeEvidenceMediaReq{
			UserId: userID, TaskId: taskID, MediaId: mediaID,
		})
		require.NoError(t, err)
		return resp.Allowed
	}

	require.True(t, allowed(reviewerID, 100, 50), "creative in the frozen snapshot")
	require.False(t, allowed(reviewerID, 100, 60), "qualification documents need the qualification role")
	require.False(t, allowed(reviewerID, 100, 0))
	require.False(t, allowed(reviewerID, 100, 999))
	require.False(t, allowed(reviewerID, 102, 50), "task outside the reviewer's market")

	fs.tasks[100].RequiredRole = store.RoleQualificationReviewer
	require.True(t, allowed(qualID, 100, 60))
	require.False(t, allowed(qualID, 100, 999))

	_, err := NewAuthorizeEvidenceMediaLogic(ctx, svcCtx).AuthorizeEvidenceMedia(&pb.AuthorizeEvidenceMediaReq{UserId: qualID, TaskId: 404})
	requireCode(t, err, errx.NotFound)
	fs.snapshots["h"] = "{"
	_, err = NewAuthorizeEvidenceMediaLogic(ctx, svcCtx).AuthorizeEvidenceMedia(&pb.AuthorizeEvidenceMediaReq{UserId: qualID, TaskId: 100})
	requireCode(t, err, errx.SystemError)
	_, err = NewAuthorizeEvidenceMediaLogic(ctx, svcCtx).AuthorizeEvidenceMedia(&pb.AuthorizeEvidenceMediaReq{UserId: adminID})
	requireCode(t, err, errx.ReviewRoleRequired)
}
