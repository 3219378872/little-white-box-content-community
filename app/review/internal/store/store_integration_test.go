//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"esx/app/review/internal/snapshot"
	"esx/pkg/event"
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/testutil"
	"esx/pkg/util"

	"github.com/stretchr/testify/require"
)

var testEnv *testutil.TestEnv

func TestMain(m *testing.M) {
	if err := util.InitSnowflake(21, 1); err != nil {
		panic(err)
	}
	testEnv = testutil.SetupTestEnvM("xbh_review", testutil.SchemaPath("xbh_review.sql"))
	code := m.Run()
	testEnv.Close()
	os.Exit(code)
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	conn := sqlx.NewSqlConnFromDB(testEnv.DB)
	for _, table := range []string{"review_task", "review_snapshot", "review_decision", "review_stage_result",
		"verdict_cache", "approved_media", "review_seed", "reviewer", "audit_log", "event_outbox", "idempotency"} {
		_, err := testEnv.DB.Exec("DELETE FROM " + table)
		require.NoError(t, err)
	}
	return New(conn)
}

func submission(objectID, revision int64, title string) (event.ReviewSubmittedEvent, snapshot.Frozen) {
	sub := event.ReviewSubmittedEvent{
		EventID: objectID*100 + revision, EventTime: 1, BizType: event.ReviewBizAdCreative,
		ObjectID: objectID, Revision: revision, Purpose: event.ReviewPurposeInitial, SubmittedAt: 1000,
		Snapshot: event.ReviewSnapshot{
			Texts: map[string]string{"title": title}, Market: "US", Language: "en", Industry: "GENERAL",
			SubmitterID: 77, Media: []event.ReviewMedia{{MediaID: 9, SHA256: "abc"}},
		},
	}
	frozen, err := snapshot.Freeze(sub.BizType, sub.Snapshot)
	if err != nil {
		panic(err)
	}
	return sub, frozen
}

func ingest(t *testing.T, s *Store, objectID, revision int64, title string, now time.Time) IngestResult {
	t.Helper()
	sub, frozen := submission(objectID, revision, title)
	result, err := s.Ingest(context.Background(), sub, frozen, "v1", now.Add(24*time.Hour).UnixMilli(), now)
	require.NoError(t, err)
	return result
}

func grantReviewer(t *testing.T, userID int64, roles string) *Reviewer {
	t.Helper()
	_, err := testEnv.DB.Exec(`INSERT INTO reviewer (user_id, roles, markets, languages, active, updated_at_ms)
		VALUES (?, ?, 'US,DE', 'en,de', 1, 1)`, userID, roles)
	require.NoError(t, err)
	return &Reviewer{UserID: userID, RolesCSV: roles, MarketsCSV: "US,DE", LanguageCSV: "en,de", Active: true}
}

// toHuman 模拟机审把任务转人审。
func toHuman(t *testing.T, s *Store, now time.Time) *Task {
	t.Helper()
	task, err := s.ClaimMachine(context.Background(), now)
	require.NoError(t, err)
	require.NotNil(t, task)
	require.NoError(t, s.Escalate(context.Background(), task, "protection", 0, now))
	return task
}

// RVW-001：重复送审与并发送审只产生一个任务。
func TestIngestIsIdempotentUnderConcurrency(t *testing.T) {
	s := newTestStore(t)
	now := time.UnixMilli(1_000_000)
	var wg sync.WaitGroup
	ids := make(chan int64, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub, frozen := submission(1, 1, "hello")
			result, err := s.Ingest(context.Background(), sub, frozen, "v1", 0, now)
			require.NoError(t, err)
			ids <- result.Task.ID
		}()
	}
	wg.Wait()
	close(ids)
	first := int64(0)
	for id := range ids {
		if first == 0 {
			first = id
		}
		require.Equal(t, first, id)
	}
	var count int
	require.NoError(t, testEnv.DB.QueryRow(`SELECT COUNT(*) FROM review_task`).Scan(&count))
	require.Equal(t, 1, count)
}

// RVW-003 / RVW-A01：新 revision 作废旧任务；乱序到达的旧 revision 直接作废；旧持有者提交被拒。
func TestNewRevisionSupersedesPendingTasks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(2_000_000)
	reviewer := grantReviewer(t, 501, RoleReviewer)
	ingest(t, s, 2, 1, "v1", now)
	toHuman(t, s, now)
	claimed, err := s.ClaimHuman(ctx, reviewer, "", now)
	require.NoError(t, err)
	require.NotNil(t, claimed)

	second := ingest(t, s, 2, 2, "v2", now)
	require.True(t, second.Created)
	_, err = s.SubmitHuman(ctx, reviewer.UserID, claimed.ID, claimed.LeaseGeneration, "k1",
		DecisionInput{Verdict: event.ReviewVerdictApprove, PolicyVersion: "v1", Source: event.ReviewSourceHuman}, now)
	require.ErrorIs(t, err, ErrTaskSuperseded)
	_, err = s.Renew(ctx, reviewer.UserID, claimed.ID, claimed.LeaseGeneration, now)
	require.ErrorIs(t, err, ErrTaskSuperseded)

	late := ingest(t, s, 2, 1, "v1-again", now) // 同键：返回已作废任务
	require.False(t, late.Created)
	require.Equal(t, StatusSuperseded, late.Task.Status)

	sub, frozen := submission(3, 2, "new")
	_, err = s.Ingest(ctx, sub, frozen, "v1", 0, now)
	require.NoError(t, err)
	oldSub, oldFrozen := submission(3, 1, "old")
	old, err := s.Ingest(ctx, oldSub, oldFrozen, "v1", 0, now)
	require.NoError(t, err)
	require.Equal(t, StatusSuperseded, old.Task.Status)
}

// RVW-A03：两名审核员竞争领取只有一人成功；持有过期被接手后，旧持有者的提交、续期与放弃均被拒绝。
func TestClaimCompetitionAndFencing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(3_000_000)
	alice := grantReviewer(t, 601, RoleReviewer)
	bob := grantReviewer(t, 602, RoleReviewer)
	ingest(t, s, 4, 1, "x", now)
	toHuman(t, s, now)

	var wg sync.WaitGroup
	results := make(chan *Task, 2)
	for _, r := range []*Reviewer{alice, bob} {
		wg.Add(1)
		go func(r *Reviewer) {
			defer wg.Done()
			task, err := s.ClaimHuman(ctx, r, "", now)
			require.NoError(t, err)
			results <- task
		}(r)
	}
	wg.Wait()
	close(results)
	var holder *Task
	got := 0
	for task := range results {
		if task != nil {
			holder = task
			got++
		}
	}
	require.Equal(t, 1, got)

	first, second := alice, bob
	if holder.LeaseHolder == bob.UserID {
		first, second = bob, alice
	}
	expired := now.Add(HumanLease + time.Second)
	_, err := s.Renew(ctx, first.UserID, holder.ID, holder.LeaseGeneration, expired)
	require.ErrorIs(t, err, ErrLeaseLost)
	taken, err := s.ClaimHuman(ctx, second, "", expired)
	require.NoError(t, err)
	require.Equal(t, holder.ID, taken.ID)
	require.Equal(t, holder.LeaseGeneration+1, taken.LeaseGeneration)

	_, err = s.SubmitHuman(ctx, first.UserID, holder.ID, holder.LeaseGeneration, "old",
		DecisionInput{Verdict: event.ReviewVerdictApprove, PolicyVersion: "v1", Source: event.ReviewSourceHuman}, expired)
	require.ErrorIs(t, err, ErrLeaseLost)
	require.ErrorIs(t, s.Release(ctx, first.UserID, holder.ID, holder.LeaseGeneration, expired), ErrLeaseLost)

	decision, err := s.SubmitHuman(ctx, second.UserID, taken.ID, taken.LeaseGeneration, "new",
		DecisionInput{Verdict: event.ReviewVerdictReject, PolicyCodes: []string{"MISLEADING.CLAIM"},
			PolicyVersion: "v1", Source: event.ReviewSourceHuman}, expired)
	require.NoError(t, err)
	again, err := s.SubmitHuman(ctx, second.UserID, taken.ID, taken.LeaseGeneration, "new",
		DecisionInput{Verdict: event.ReviewVerdictReject, PolicyCodes: []string{"MISLEADING.CLAIM"},
			PolicyVersion: "v1", Source: event.ReviewSourceHuman}, expired)
	require.NoError(t, err)
	require.Equal(t, decision.ID, again.ID)
	_, err = s.SubmitHuman(ctx, second.UserID, taken.ID, taken.LeaseGeneration, "different",
		DecisionInput{Verdict: event.ReviewVerdictApprove, PolicyVersion: "v1", Source: event.ReviewSourceHuman}, expired)
	require.ErrorIs(t, err, ErrTaskDecided)

	var outbox, audits int
	require.NoError(t, testEnv.DB.QueryRow(`SELECT COUNT(*) FROM event_outbox WHERE topic = 'review-decided'`).Scan(&outbox))
	require.Equal(t, 1, outbox)
	require.NoError(t, testEnv.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE object_id = ?`, holder.ID).Scan(&audits))
	require.GreaterOrEqual(t, audits, 3) // claim、claim、submit
	cached, err := s.LookupVerdict(ctx, taken.SnapshotHash, "US", "v1")
	require.NoError(t, err)
	require.Equal(t, event.ReviewVerdictReject, cached.Verdict)
}

// RVW-022：只领取授权市场与语言；RVW-024：质检排除原决策人；RVW-014：自动通过可建质检任务。
func TestClaimRespectsScopeAndQAExclusion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(4_000_000)
	reviewer := grantReviewer(t, 701, RoleReviewer+","+RoleQA)
	idOnly := &Reviewer{UserID: 702, RolesCSV: RoleReviewer, MarketsCSV: "ID", LanguageCSV: "id", Active: true}
	ingest(t, s, 5, 1, "x", now)
	toHuman(t, s, now)
	none, err := s.ClaimHuman(ctx, idOnly, "", now)
	require.NoError(t, err)
	require.Nil(t, none)

	task, err := s.ClaimHuman(ctx, reviewer, "", now)
	require.NoError(t, err)
	_, err = s.SubmitHuman(ctx, reviewer.UserID, task.ID, task.LeaseGeneration, "",
		DecisionInput{Verdict: event.ReviewVerdictApprove, PolicyVersion: "v1", Source: event.ReviewSourceHuman}, now)
	require.NoError(t, err)

	machine := ingest(t, s, 6, 1, "auto", now)
	claimed, err := s.ClaimMachine(ctx, now)
	require.NoError(t, err)
	require.Equal(t, machine.Task.ID, claimed.ID)
	_, err = s.DecideMachine(ctx, claimed, DecisionInput{Verdict: event.ReviewVerdictApprove, PolicyVersion: "v1",
		Source: event.ReviewSourceMachine, CreateQA: true}, now.Add(time.Hour).UnixMilli(), now)
	require.NoError(t, err)
	qa, err := s.ClaimHuman(ctx, reviewer, event.ReviewPurposeQA, now)
	require.NoError(t, err)
	require.NotNil(t, qa)
	require.Equal(t, RoleQA, qa.RequiredRole)
	require.Equal(t, claimed.ID, qa.SourceTaskID)

	machineCached, err := s.LookupVerdict(ctx, claimed.SnapshotHash, "US", "v1")
	require.NoError(t, err)
	require.Nil(t, machineCached) // RVW-015：机审通过不可复用
	cached, err := s.LookupVerdict(ctx, task.SnapshotHash, "US", "v1")
	require.NoError(t, err)
	require.NotNil(t, cached)
	require.Equal(t, event.ReviewSourceHuman, cached.Source)
	media, err := s.ApprovedMedia(ctx, []string{"abc", "zzz"})
	require.NoError(t, err)
	require.True(t, media["abc"])
	require.False(t, media["zzz"])
}

// RVW-030：提名人不能确认自己的种子；停用后不再是 active。
func TestSeedRequiresSecondPerson(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(5_000_000)
	reviewer := grantReviewer(t, 801, RoleReviewer)
	ingest(t, s, 7, 1, "double your bitcoin", now)
	toHuman(t, s, now)
	task, err := s.ClaimHuman(ctx, reviewer, "", now)
	require.NoError(t, err)
	_, err = s.SubmitHuman(ctx, reviewer.UserID, task.ID, task.LeaseGeneration, "",
		DecisionInput{Verdict: event.ReviewVerdictReject, PolicyCodes: []string{"CONTENT.DECEPTIVE"}, PolicyVersion: "v1",
			Source: event.ReviewSourceHuman, NominateSeed: true, SeedText: "double your bitcoin"}, now)
	require.NoError(t, err)
	seeds, err := s.ListSeeds(ctx, SeedCandidate, 10)
	require.NoError(t, err)
	require.Len(t, seeds, 1)
	_, err = s.TransitionSeed(ctx, seeds[0].ID, reviewer.UserID, SeedActive, now)
	require.ErrorIs(t, err, ErrSameActor)
	confirmed, err := s.TransitionSeed(ctx, seeds[0].ID, 999, SeedActive, now)
	require.NoError(t, err)
	require.Equal(t, SeedActive, confirmed.Status)
	_, err = s.TransitionSeed(ctx, seeds[0].ID, 999, SeedRetired, now)
	require.NoError(t, err)
	statuses, err := s.SeedStatuses(ctx, []int64{seeds[0].ID})
	require.NoError(t, err)
	require.Equal(t, SeedRetired, statuses[seeds[0].ID])
	_, err = s.TransitionSeed(ctx, seeds[0].ID, 999, SeedActive, now)
	require.True(t, errors.Is(err, ErrSeedTransition))
}

// RVW-050 / RVW-A05：撤销后读取立即反映；授予与撤销写审计。
func TestGrantAndRevokeRolesAreAudited(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(6_000_000)
	require.Error(t, s.GrantRoles(ctx, 0, Grant{UserID: 1, Roles: []string{"admin"}}, now))
	require.NoError(t, s.GrantRoles(ctx, 0, Grant{UserID: 1, Roles: []string{RoleReviewer, RoleQA},
		Markets: []string{"US"}, Languages: []string{"en"}}, now))
	r, err := s.Reviewer(ctx, 1)
	require.NoError(t, err)
	require.True(t, r.HasRole(RoleQA))
	require.NoError(t, s.RevokeRoles(ctx, 0, 1, now))
	r, err = s.Reviewer(ctx, 1)
	require.NoError(t, err)
	require.False(t, r.HasRole(RoleReviewer))
	require.ErrorIs(t, s.RevokeRoles(ctx, 0, 999, now), ErrReviewerUnknown)
	var audits int
	require.NoError(t, testEnv.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE object_type = 'reviewer' AND object_id = 1`).Scan(&audits))
	require.Equal(t, 2, audits)
}
