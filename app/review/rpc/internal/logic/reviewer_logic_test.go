package logic

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"esx/app/review/internal/store"
	pb "esx/kitex_gen/review"
	"esx/pkg/adpolicy"
	"esx/pkg/errx"
	"esx/pkg/event"
)

func TestGetReviewer(t *testing.T) {
	ctx := context.Background()
	svcCtx, fs := newFixture()
	l := NewGetReviewerLogic(ctx, svcCtx)

	_, err := l.GetReviewer(&pb.GetReviewerReq{})
	requireCode(t, err, errx.LoginRequired)

	resp, err := l.GetReviewer(&pb.GetReviewerReq{UserId: 99})
	require.NoError(t, err)
	require.False(t, resp.Active, "unknown users simply have no reviewer entry")

	resp, err = l.GetReviewer(&pb.GetReviewerReq{UserId: inactiveID})
	require.NoError(t, err)
	require.False(t, resp.Active)

	resp, err = l.GetReviewer(&pb.GetReviewerReq{UserId: reviewerID})
	require.NoError(t, err)
	require.True(t, resp.Active)
	require.Equal(t, []string{"reviewer", "qa"}, resp.Roles)
	require.Equal(t, []string{"US"}, resp.Markets)
	require.Equal(t, []string{"en"}, resp.Languages)

	fs.reviewerErr = errBoom
	_, err = l.GetReviewer(&pb.GetReviewerReq{UserId: reviewerID})
	requireCode(t, err, errx.SystemError)
}

func TestGetQueueSummary(t *testing.T) {
	ctx := context.Background()
	svcCtx, fs := newFixture()
	fs.buckets = []store.QueueBucket{{Purpose: event.ReviewPurposeInitial, Pending: 3, OldestAgeMs: 1000}}

	resp, err := NewGetQueueSummaryLogic(ctx, svcCtx).GetQueueSummary(&pb.GetQueueSummaryReq{UserId: reviewerID})
	require.NoError(t, err)
	require.Equal(t, "demo-v1", resp.PolicyVersion)
	require.Equal(t, []*pb.QueueBucket{{Purpose: event.ReviewPurposeInitial, Pending: 3, OldestAgeMs: 1000}}, resp.Buckets)

	fs.queueErr = errBoom
	_, err = NewGetQueueSummaryLogic(ctx, svcCtx).GetQueueSummary(&pb.GetQueueSummaryReq{UserId: reviewerID})
	requireCode(t, err, errx.SystemError)
	_, err = NewGetQueueSummaryLogic(ctx, svcCtx).GetQueueSummary(&pb.GetQueueSummaryReq{UserId: adminID})
	requireCode(t, err, errx.ReviewRoleRequired)
}

func TestListPolicies(t *testing.T) {
	svcCtx, _ := newFixture()
	resp, err := NewListPoliciesLogic(context.Background(), svcCtx).ListPolicies(&pb.ListPoliciesReq{})
	require.NoError(t, err)
	require.Equal(t, "demo-v1", resp.PolicyVersion)
	require.Len(t, resp.Codes, len(adpolicy.Codes))
	require.Len(t, resp.Markets, len(adpolicy.Markets))
}

func TestSeeds(t *testing.T) {
	ctx := context.Background()
	svcCtx, fs := newFixture()
	fs.seeds = []store.Seed{*fs.seed}

	resp, err := NewListSeedsLogic(ctx, svcCtx).ListSeeds(&pb.ListSeedsReq{UserId: adminID})
	require.NoError(t, err)
	require.Equal(t, store.SeedCandidate, fs.seedStatus, "status defaults to candidate")
	require.Len(t, resp.Seeds, 1)
	require.Equal(t, "CONTENT.IP", resp.Seeds[0].IssueCode)

	_, err = NewListSeedsLogic(ctx, svcCtx).ListSeeds(&pb.ListSeedsReq{UserId: adminID, Status: "deleted"})
	requireCode(t, err, errx.ParamError)
	_, err = NewListSeedsLogic(ctx, svcCtx).ListSeeds(&pb.ListSeedsReq{UserId: reviewerID})
	requireCode(t, err, errx.ReviewRoleRequired)
	fs.seedsErr = errBoom
	_, err = NewListSeedsLogic(ctx, svcCtx).ListSeeds(&pb.ListSeedsReq{UserId: adminID, Status: store.SeedActive})
	requireCode(t, err, errx.SystemError)

	seed, err := NewConfirmSeedLogic(ctx, svcCtx).ConfirmSeed(&pb.SeedActionReq{UserId: adminID, SeedId: 30})
	require.NoError(t, err)
	require.Equal(t, store.SeedActive, seed.Seed.Status)
	seed, err = NewRetireSeedLogic(ctx, svcCtx).RetireSeed(&pb.SeedActionReq{UserId: adminID, SeedId: 30})
	require.NoError(t, err)
	require.Equal(t, store.SeedRetired, seed.Seed.Status)

	fs.transitionErr = store.ErrSameActor
	_, err = NewConfirmSeedLogic(ctx, svcCtx).ConfirmSeed(&pb.SeedActionReq{UserId: adminID, SeedId: 30})
	requireCode(t, err, errx.PermissionDenied)
	_, err = NewRetireSeedLogic(ctx, svcCtx).RetireSeed(&pb.SeedActionReq{UserId: reviewerID, SeedId: 30})
	requireCode(t, err, errx.ReviewRoleRequired)
}

func TestEnsureSubmitted(t *testing.T) {
	ctx := context.Background()
	svcCtx, _ := newFixture()
	ingester := &fakeIngester{result: store.IngestResult{Task: &store.Task{ID: 5, Status: "machine_pending"}, Created: true}}
	svcCtx.Ingester = ingester
	sub := event.ReviewSubmittedEvent{
		BizType: event.ReviewBizAdCreative, ObjectID: 7, Revision: 1, Purpose: event.ReviewPurposeInitial,
		SubmittedAt: 1, Snapshot: event.ReviewSnapshot{Market: "US", SubmitterID: 9},
	}

	resp, err := NewEnsureSubmittedLogic(ctx, svcCtx).EnsureSubmitted(&pb.EnsureSubmittedReq{SubmissionJson: mustJSON(sub)})
	require.NoError(t, err)
	require.Equal(t, &pb.EnsureSubmittedResp{TaskId: 5, Created: true, Status: "machine_pending"}, resp)

	_, err = NewEnsureSubmittedLogic(ctx, svcCtx).EnsureSubmitted(&pb.EnsureSubmittedReq{SubmissionJson: "{"})
	requireCode(t, err, errx.ParamError)

	ingester.err = errBoom
	_, err = NewEnsureSubmittedLogic(ctx, svcCtx).EnsureSubmitted(&pb.EnsureSubmittedReq{SubmissionJson: mustJSON(sub)})
	requireCode(t, err, errx.SystemError)
}
