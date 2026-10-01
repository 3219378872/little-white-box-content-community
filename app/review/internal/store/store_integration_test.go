//go:build integration

package store

import (
	"context"
	"encoding/json"
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

func followUp(t *testing.T, s *Store, objectID, revision int64, purpose, key string, priority int32, now time.Time) IngestResult {
	t.Helper()
	sub, frozen := submission(objectID, revision, "serving")
	sub.Purpose, sub.PurposeKey, sub.Priority = purpose, key, priority
	result, err := s.Ingest(context.Background(), sub, frozen, "v1", now.Add(24*time.Hour).UnixMilli(), now)
	require.NoError(t, err)
	return result
}

func decideHuman(t *testing.T, s *Store, reviewer *Reviewer, purpose, verdict string, codes []string, now time.Time) *Task {
	t.Helper()
	task, err := s.ClaimHuman(context.Background(), reviewer, purpose, now)
	require.NoError(t, err)
	require.NotNil(t, task, "no %s task for reviewer %d", purpose, reviewer.UserID)
	_, err = s.SubmitHuman(context.Background(), reviewer.UserID, task.ID, task.LeaseGeneration, "", DecisionInput{
		Verdict: verdict, PolicyCodes: codes, PolicyVersion: "v1", Source: event.ReviewSourceHuman,
	}, now)
	require.NoError(t, err)
	return task
}

// RVW-024 / ADS-014：申诉直接进入人审，以最近的拒绝为原结论并排除其决策人。
func TestAppealExcludesOriginalDecider(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(7_000_000)
	alice := grantReviewer(t, 901, RoleReviewer)
	bob := grantReviewer(t, 902, RoleReviewer)
	ingest(t, s, 20, 1, "claim", now)
	toHuman(t, s, now)
	original := decideHuman(t, s, alice, "", event.ReviewVerdictReject, []string{"MISLEADING.CLAIM"}, now)

	appeal := followUp(t, s, 20, 1, event.ReviewPurposeAppeal, "", 60, now)
	require.True(t, appeal.Created)
	require.Equal(t, StatusHumanPending, appeal.Task.Status)
	require.Equal(t, original.ID, appeal.Task.SourceTaskID)
	require.Equal(t, alice.UserID, appeal.Task.ExcludedReviewer)

	none, err := s.ClaimHuman(ctx, alice, event.ReviewPurposeAppeal, now)
	require.NoError(t, err)
	require.Nil(t, none)
	decideHuman(t, s, bob, event.ReviewPurposeAppeal, event.ReviewVerdictApprove, nil, now)
	again := followUp(t, s, 20, 1, event.ReviewPurposeAppeal, "", 60, now)
	require.False(t, again.Created, "每个 revision 只有一次申诉")
}

// ADS-030：同一批举报只建一个任务，后续举报提高优先级；举报直接进入人审。
func TestReportBatchRaisesPriority(t *testing.T) {
	s := newTestStore(t)
	now := time.UnixMilli(8_000_000)
	first := followUp(t, s, 21, 1, event.ReviewPurposeReport, "batch-1", 60, now)
	require.True(t, first.Created)
	require.Equal(t, StatusHumanPending, first.Task.Status)
	require.Equal(t, event.ReviewPurposeReport, first.Task.EscalationReason)

	more := followUp(t, s, 21, 1, event.ReviewPurposeReport, "batch-1", 80, now)
	require.False(t, more.Created)
	require.Equal(t, first.Task.ID, more.Task.ID)
	lower := followUp(t, s, 21, 1, event.ReviewPurposeReport, "batch-1", 70, now)
	require.Equal(t, 80, lower.Task.Priority)
	stored, err := s.GetTask(context.Background(), first.Task.ID)
	require.NoError(t, err)
	require.Equal(t, 80, stored.Priority)

	next := followUp(t, s, 21, 1, event.ReviewPurposeReport, "batch-2", 60, now)
	require.True(t, next.Created)
}

// ADS-011 / RVW-003：投后任务针对在投的过审快照，不因更新的未决 revision 作废；更新的 revision
// 过审后才作废，此后到达的旧快照投后任务直接作废。送审与申诉仍按 revision 作废。
func TestPostServingTasksFollowTheServingRevision(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(9_000_000)
	reviewer := grantReviewer(t, 1001, RoleReviewer)
	report := followUp(t, s, 22, 1, event.ReviewPurposeReport, "b", 60, now)
	rescan := followUp(t, s, 22, 1, event.ReviewPurposeRescan, "v2+seeds@1", 0, now)

	ingest(t, s, 22, 2, "edit", now)
	for _, id := range []int64{report.Task.ID, rescan.Task.ID} {
		task, err := s.GetTask(ctx, id)
		require.NoError(t, err)
		require.NotEqual(t, StatusSuperseded, task.Status)
	}

	claimed, err := s.ClaimMachine(ctx, now)
	require.NoError(t, err)
	require.Equal(t, rescan.Task.ID, claimed.ID, "回扫先于更新 revision 的送审被领取（同优先级按 ID）")
	require.NoError(t, s.Escalate(ctx, claimed, "gray-zone", 10, now))
	edit, err := s.ClaimMachine(ctx, now)
	require.NoError(t, err)
	require.Equal(t, int64(2), edit.ObjectRevision)
	_, err = s.DecideMachine(ctx, edit, DecisionInput{Verdict: event.ReviewVerdictApprove, PolicyVersion: "v1",
		Source: event.ReviewSourceMachine}, 0, now)
	require.NoError(t, err)

	for _, id := range []int64{report.Task.ID, rescan.Task.ID} {
		task, err := s.GetTask(ctx, id)
		require.NoError(t, err)
		require.Equal(t, StatusSuperseded, task.Status)
	}
	late := followUp(t, s, 22, 1, event.ReviewPurposeRescan, "v3+seeds@1", 0, now)
	require.Equal(t, StatusSuperseded, late.Task.Status)
	none, err := s.ClaimHuman(ctx, reviewer, "", now)
	require.NoError(t, err)
	require.Nil(t, none)
}

// ADS-031：回扫判定违规同事务转人审并下发暂停结论；fencing 失败时不下发。
func TestRescanViolationPausesAndEscalates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(10_000_000)
	followUp(t, s, 23, 1, event.ReviewPurposeRescan, "v2+seeds@0", 0, now)
	task, err := s.ClaimMachine(ctx, now)
	require.NoError(t, err)

	stale := *task
	stale.LeaseGeneration--
	require.ErrorIs(t, s.FlagRescanViolation(ctx, &stale, []string{"CONTENT.DECEPTIVE"}, "v2", 90, now), ErrLeaseLost)
	require.NoError(t, s.FlagRescanViolation(ctx, task, []string{"CONTENT.DECEPTIVE"}, "v2", 90, now))
	require.ErrorIs(t, s.FlagRescanViolation(ctx, task, []string{"CONTENT.DECEPTIVE"}, "v2", 90, now), ErrLeaseLost)

	stored, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, StatusHumanPending, stored.Status)
	require.Equal(t, RescanPauseReason, stored.EscalationReason)
	require.Equal(t, 90, stored.Priority)
	require.Zero(t, stored.DecisionID)

	var payload []byte
	require.NoError(t, testEnv.DB.QueryRow(`SELECT payload FROM event_outbox WHERE topic = 'review-decided'`).Scan(&payload))
	var interim event.ReviewDecidedEvent
	require.NoError(t, json.Unmarshal(payload, &interim))
	require.NoError(t, interim.Validate())
	require.True(t, interim.Interim)
	require.Equal(t, task.ID, interim.TaskID)
	require.Equal(t, []string{"CONTENT.DECEPTIVE"}, interim.PolicyCodes)

	reviewer := grantReviewer(t, 1101, RoleReviewer)
	final := decideHuman(t, s, reviewer, event.ReviewPurposeRescan, event.ReviewVerdictReject, []string{"CONTENT.DECEPTIVE"}, now)
	require.Equal(t, task.ID, final.ID)
	var outbox int
	require.NoError(t, testEnv.DB.QueryRow(`SELECT COUNT(*) FROM event_outbox WHERE topic = 'review-decided'`).Scan(&outbox))
	require.Equal(t, 2, outbox)
}

// ADS-031：回扫代次来自生效种子变化与政策激活审计。
func TestRescanGenerationSources(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(11_000_000)
	_, found, err := s.PolicyActivatedAt(ctx, "ads_v%")
	require.NoError(t, err)
	require.False(t, found)
	for i, version := range []string{"ads-1", "ads-2", "ads-1"} {
		require.NoError(t, s.Audit(ctx, AuditEntry{Action: "policy.activate", ObjectType: "policy",
			After: map[string]any{"version": version, "configHash": "h", "mode": "active"}}, now.Add(time.Duration(i)*time.Minute)))
	}
	require.NoError(t, s.Audit(ctx, AuditEntry{Action: "policy.activate", ObjectType: "policy",
		After: map[string]any{"version": "ads-3", "configHash": "h", "mode": "shadow"}}, now))
	at, found, err := s.PolicyActivatedAt(ctx, "ads-1")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, now.UnixMilli(), at)
	_, found, err = s.PolicyActivatedAt(ctx, "ads-3")
	require.NoError(t, err)
	require.False(t, found, "影子激活不是生效版本")

	changes, _, err := s.SeedGeneration(ctx)
	require.NoError(t, err)
	require.Zero(t, changes)
	reviewer := grantReviewer(t, 1201, RoleReviewer)
	for objectID, text := range map[int64]string{30: "double your bitcoin", 31: "free crypto"} {
		ingest(t, s, objectID, 1, text, now)
		toHuman(t, s, now)
		task, err := s.ClaimHuman(ctx, reviewer, "", now)
		require.NoError(t, err)
		_, err = s.SubmitHuman(ctx, reviewer.UserID, task.ID, task.LeaseGeneration, "", DecisionInput{
			Verdict: event.ReviewVerdictReject, PolicyCodes: []string{"CONTENT.DECEPTIVE"}, PolicyVersion: "v1",
			Source: event.ReviewSourceHuman, NominateSeed: true, SeedText: text}, now)
		require.NoError(t, err)
	}
	seeds, err := s.ListSeeds(ctx, SeedCandidate, 10)
	require.NoError(t, err)
	require.Len(t, seeds, 2)
	_, err = s.TransitionSeed(ctx, seeds[0].ID, 999, SeedRetired, now) // 候选直接停用不改变生效集合
	require.NoError(t, err)
	changes, _, err = s.SeedGeneration(ctx)
	require.NoError(t, err)
	require.Zero(t, changes)
	later := now.Add(time.Hour)
	_, err = s.TransitionSeed(ctx, seeds[1].ID, 999, SeedActive, later)
	require.NoError(t, err)
	changes, changedAt, err := s.SeedGeneration(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), changes)
	require.Equal(t, later.UnixMilli(), changedAt)
	_, err = s.TransitionSeed(ctx, seeds[1].ID, 999, SeedRetired, later.Add(time.Minute))
	require.NoError(t, err)
	changes, changedAt, err = s.SeedGeneration(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), changes)
	require.Equal(t, later.Add(time.Minute).UnixMilli(), changedAt)
}
