package cascade

import (
	"context"
	"errors"
	"testing"
	"time"

	"esx/app/review/internal/policy"
	"esx/pkg/event"

	"github.com/stretchr/testify/require"
)

type fakeRouter struct {
	result RouteResult
	err    error
}

func (f *fakeRouter) Route(context.Context, RouteInput) (RouteResult, error) { return f.result, f.err }
func (f *fakeRouter) Version() string                                        { return "seedbank@1+fake" }

type fakeRanker struct {
	result RankResult
	err    error
	block  bool
	calls  int
	issues []string
}

func (f *fakeRanker) Score(ctx context.Context, in RankInput) (RankResult, error) {
	f.calls++
	f.issues = in.Issues
	if f.block {
		<-ctx.Done()
		return RankResult{}, ctx.Err()
	}
	return f.result, f.err
}

type fakeLookup struct {
	verdict, source string
	codes           []string
	approvedMedia   map[string]bool
}

func (f *fakeLookup) LookupVerdict(context.Context, string, string, string) (string, []string, string, bool, error) {
	return f.verdict, f.codes, f.source, f.verdict != "", nil
}

func (f *fakeLookup) ApprovedMedia(_ context.Context, hashes []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, h := range hashes {
		out[h] = f.approvedMedia[h]
	}
	return out, nil
}

func loadPolicy(t *testing.T) *policy.Policy {
	t.Helper()
	p, err := policy.Load("ads-2026-10-01")
	require.NoError(t, err)
	return p
}

func scoresFor(issues []string, pYes float64) RankResult {
	out := RankResult{ModelVersion: "fake-ranker@1"}
	for _, issue := range issues {
		out.Scores = append(out.Scores, IssueScore{Issue: issue, PYes: pYes, PNo: 1 - pYes})
	}
	return out
}

func baseInput() Input {
	return Input{
		TaskID: 1, BizType: event.ReviewBizAdCreative, Purpose: event.ReviewPurposeInitial,
		SnapshotHash: "h", SubmissionSeq: 4,
		Snapshot: event.ReviewSnapshot{
			Texts:  map[string]string{"title": "Fresh coffee beans", "body": "Roasted weekly"},
			Market: "US", Language: "en", Industry: "GENERAL", SubmitterID: 1,
			LandingURL: "https://coffee.shop.com/beans",
		},
	}
}

func coveredRouter(sim float64, issues ...string) *fakeRouter {
	similarity := map[string]float64{}
	for _, issue := range issues {
		similarity[issue] = sim
	}
	return &fakeRouter{result: RouteResult{Version: "seedbank@1+fake", Similarity: similarity, Covered: true}}
}

func stage(result Result, name string) StageRecord {
	for _, s := range result.Stages {
		if s.Stage == name {
			return s
		}
	}
	return StageRecord{}
}

func TestAutoPassRequiresEveryCondition(t *testing.T) {
	p := loadPolicy(t)
	c := &Cascade{Router: coveredRouter(0.1), Ranker: &fakeRanker{}, Lookup: &fakeLookup{}}
	result := c.Run(context.Background(), p, baseInput())
	require.Equal(t, OutcomeApprove, result.Outcome)
	require.Equal(t, "no-candidates", stage(result, StageRanker).Reason)

	protected := baseInput()
	protected.SubmissionSeq = 3
	require.Equal(t, ReasonProtection, c.Run(context.Background(), p, protected).Reason)

	financial := baseInput()
	financial.Snapshot.Industry = "FINANCIAL"
	require.Equal(t, ReasonIndustry, c.Run(context.Background(), p, financial).Reason)

	image := baseInput()
	image.Snapshot.Media = []event.ReviewMedia{{MediaID: 1, SHA256: "new"}}
	require.Equal(t, ReasonImageUnconfirmed, c.Run(context.Background(), p, image).Reason)
	c.Lookup = &fakeLookup{approvedMedia: map[string]bool{"new": true}}
	require.Equal(t, OutcomeApprove, c.Run(context.Background(), p, image).Outcome)
}

func TestHardRulesRejectWithPolicyCodes(t *testing.T) {
	p := loadPolicy(t)
	c := &Cascade{Router: coveredRouter(0.1), Ranker: &fakeRanker{}}
	cases := map[string]struct {
		edit func(*Input)
		code string
	}{
		"alcohol":        {func(in *Input) { in.Snapshot.Industry = "ALCOHOL" }, "INDUSTRY.ALCOHOL"},
		"gambling":       {func(in *Input) { in.Snapshot.Industry = "GAMBLING"; in.Snapshot.Market = "DE" }, "INDUSTRY.GAMBLING"},
		"weight in ID":   {func(in *Input) { in.Snapshot.Industry = "WEIGHT"; in.Snapshot.Market = "ID" }, "INDUSTRY.WEIGHT"},
		"plain http":     {func(in *Input) { in.Snapshot.LandingURL = "http://coffee.shop.com" }, "LANDING.URL"},
		"blocked domain": {func(in *Input) { in.Snapshot.LandingURL = "https://win.scam-giveaway.com/x" }, "LANDING.DOMAIN"},
		"keyword":        {func(in *Input) { in.Snapshot.Texts["body"] = "Double   your BITCOIN today" }, "CONTENT.DECEPTIVE"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			in := baseInput()
			tc.edit(&in)
			result := c.Run(context.Background(), p, in)
			require.Equal(t, OutcomeReject, result.Outcome)
			require.Contains(t, result.PolicyCodes, tc.code)
		})
	}
}

func TestForcedHumanRuleBlocksAutoPass(t *testing.T) {
	p := loadPolicy(t)
	in := baseInput()
	in.Snapshot.Texts["body"] = "Guaranteed results in a week"
	result := (&Cascade{Router: coveredRouter(0.1), Ranker: &fakeRanker{}}).Run(context.Background(), p, in)
	require.Equal(t, OutcomeHuman, result.Outcome)
	require.Equal(t, ReasonForcedRule, result.Reason)
	require.Contains(t, result.PolicyCodes, "MISLEADING.CLAIM")
}

// RVW-A02：召回与精排的超时、不可用、无效输出都不放宽结论，并在阶段记录中标明降级。
func TestDegradationNeverLoosensDecisions(t *testing.T) {
	p := loadPolicy(t)
	ctx := context.Background()

	t.Run("router unavailable ranks every issue", func(t *testing.T) {
		ranker := &fakeRanker{}
		ranker.result = scoresFor(p.Issues, 0.5)
		c := &Cascade{Router: &fakeRouter{err: ErrUnavailable}, Ranker: ranker}
		result := c.Run(ctx, p, baseInput())
		require.Equal(t, OutcomeHuman, result.Outcome)
		require.Equal(t, ReasonGrayZone, result.Reason)
		require.ElementsMatch(t, p.Issues, ranker.issues)
		require.Equal(t, ReasonRouterUnavailable, stage(result, StageRouter).Reason)
	})
	t.Run("router without seed coverage ranks every issue", func(t *testing.T) {
		ranker := &fakeRanker{result: scoresFor(p.Issues, 0.01)}
		c := &Cascade{Router: &fakeRouter{result: RouteResult{Version: "v", Covered: false}}, Ranker: ranker}
		result := c.Run(ctx, p, baseInput())
		require.ElementsMatch(t, p.Issues, ranker.issues)
		require.Equal(t, ReasonRouterNoCoverage, stage(result, StageRouter).Reason)
		require.Equal(t, OutcomeApprove, result.Outcome) // 全部 issue 精排低于通过阈值，条件未放宽
	})
	t.Run("ranker timeout", func(t *testing.T) {
		started := time.Now()
		c := &Cascade{Router: &fakeRouter{err: ErrUnavailable}, Ranker: &fakeRanker{block: true}}
		result := c.Run(ctx, p, baseInput())
		require.Equal(t, OutcomeHuman, result.Outcome)
		require.Equal(t, ReasonRankerTimeout, result.Reason)
		require.Less(t, time.Since(started), RankerTimeout+time.Second)
	})
	t.Run("ranker unavailable", func(t *testing.T) {
		c := &Cascade{Router: coveredRouter(0.99, "MISLEADING.CLAIM"), Ranker: &fakeRanker{err: ErrUnavailable}}
		result := c.Run(ctx, p, baseInput())
		require.Equal(t, OutcomeHuman, result.Outcome)
		require.Equal(t, ReasonRankerUnavailable, stage(result, StageRanker).Reason)
	})
	t.Run("ranker missing an issue", func(t *testing.T) {
		ranker := &fakeRanker{result: RankResult{ModelVersion: "m"}}
		c := &Cascade{Router: coveredRouter(0.99, "MISLEADING.CLAIM"), Ranker: ranker}
		require.Equal(t, ReasonRankerInvalid, c.Run(ctx, p, baseInput()).Reason)
	})
	t.Run("nil components", func(t *testing.T) {
		result := (&Cascade{}).Run(ctx, p, baseInput())
		require.Equal(t, OutcomeHuman, result.Outcome)
		require.Equal(t, ReasonRankerUnavailable, result.Reason)
	})
}

func TestRankerScoresDriveRejectAndGrayZone(t *testing.T) {
	p := loadPolicy(t)
	ctx := context.Background()
	// CONTENT.SELF_HARM 允许自动拒绝且阈值 0.90。
	ranker := &fakeRanker{result: scoresFor([]string{"CONTENT.SELF_HARM"}, 0.95)}
	c := &Cascade{Router: coveredRouter(0.9, "CONTENT.SELF_HARM"), Ranker: ranker}
	result := c.Run(ctx, p, baseInput())
	require.Equal(t, OutcomeReject, result.Outcome)
	require.Equal(t, []string{"CONTENT.SELF_HARM"}, result.PolicyCodes)
	// MISLEADING.CLAIM 不允许自动拒绝：高分只进人审。
	ranker = &fakeRanker{result: scoresFor([]string{"MISLEADING.CLAIM"}, 0.99)}
	c = &Cascade{Router: coveredRouter(0.9, "MISLEADING.CLAIM"), Ranker: ranker}
	result = c.Run(ctx, p, baseInput())
	require.Equal(t, OutcomeHuman, result.Outcome)
	require.Equal(t, 99, result.Priority)
}

// RVW-015 / RVW-A04：指纹复用的拒绝直接复用，通过只来自人工或质检且受保护期约束。
func TestFingerprintReuse(t *testing.T) {
	p := loadPolicy(t)
	ctx := context.Background()
	router := coveredRouter(0.1)
	reject := &Cascade{Router: router, Lookup: &fakeLookup{verdict: event.ReviewVerdictReject, codes: []string{"CONTENT.IP"}, source: event.ReviewSourceMachine}}
	result := reject.Run(ctx, p, baseInput())
	require.Equal(t, OutcomeReject, result.Outcome)
	require.Equal(t, []string{"CONTENT.IP"}, result.PolicyCodes)

	approve := &Cascade{Router: router, Ranker: &fakeRanker{}, Lookup: &fakeLookup{verdict: event.ReviewVerdictApprove, source: event.ReviewSourceHuman}}
	require.Equal(t, "fingerprint-approve", approve.Run(ctx, p, baseInput()).Reason)
	protected := baseInput()
	protected.SubmissionSeq = 1
	require.Equal(t, ReasonProtection, approve.Run(ctx, p, protected).Reason)

	machineApprove := &Cascade{Router: router, Ranker: &fakeRanker{}, Lookup: &fakeLookup{verdict: event.ReviewVerdictApprove, source: event.ReviewSourceMachine}}
	require.Equal(t, "miss", stage(machineApprove.Run(ctx, p, baseInput()), StageFingerprint).Outcome)
}

func TestQualificationAndHumanPurposesSkipAutomation(t *testing.T) {
	p := loadPolicy(t)
	in := baseInput()
	in.BizType = event.ReviewBizAdvertiserQualification
	result := (&Cascade{}).Run(context.Background(), p, in)
	require.Equal(t, OutcomeHuman, result.Outcome)
	require.Equal(t, ReasonQualification, result.Reason)
	in = baseInput()
	in.Purpose = event.ReviewPurposeAppeal
	require.Equal(t, ReasonPurpose, (&Cascade{}).Run(context.Background(), p, in).Reason)
}

func TestValidateRank(t *testing.T) {
	issues := []string{"A", "B"}
	require.NoError(t, ValidateRank(issues, RankResult{ModelVersion: "m", Scores: []IssueScore{{"A", 0.2, 0.8}, {"B", 0.6, 0.4}}}))
	bad := []RankResult{
		{Scores: []IssueScore{{"A", 0.2, 0.8}, {"B", 0.6, 0.4}}},
		{ModelVersion: "m", Scores: []IssueScore{{"A", 0.2, 0.8}}},
		{ModelVersion: "m", Scores: []IssueScore{{"A", 1.2, -0.2}, {"B", 0.6, 0.4}}},
		{ModelVersion: "m", Scores: []IssueScore{{"A", 0.2, 0.2}, {"B", 0.6, 0.4}}},
		{ModelVersion: "m", Scores: []IssueScore{{"A", 0.2, 0.8}, {"A", 0.2, 0.8}}},
		{ModelVersion: "m", Scores: []IssueScore{{"A", 0.2, 0.8}, {"B", 0.6, 0.4}, {"C", 0.1, 0.9}}},
	}
	for i, result := range bad {
		require.Error(t, ValidateRank(issues, result), i)
	}
	require.True(t, errors.Is(ErrUnavailable, ErrUnavailable))
}

// ADS-031：回扫判定违规不受自动拒绝开关限制，强制规则与灰区转人审核实，其余视为未发现违规。
func TestRescanJudgesViolationNotApproval(t *testing.T) {
	p := loadPolicy(t)
	rescan := func(edit func(*Input)) Input {
		in := baseInput()
		in.Purpose = event.ReviewPurposeRescan
		in.SubmissionSeq = 0
		if edit != nil {
			edit(&in)
		}
		return in
	}

	// MISLEADING.CLAIM 在 US 不允许自动拒绝；首次审核只会进人审，回扫则判定违规。
	ranker := &fakeRanker{result: scoresFor([]string{"MISLEADING.CLAIM"}, 0.97)}
	c := &Cascade{Router: coveredRouter(0.9, "MISLEADING.CLAIM"), Ranker: ranker}
	initial := c.Run(context.Background(), p, baseInput())
	require.Equal(t, OutcomeHuman, initial.Outcome)
	violation := c.Run(context.Background(), p, rescan(nil))
	require.Equal(t, OutcomeReject, violation.Outcome)
	require.Equal(t, ReasonRescanViolation, violation.Reason)
	require.Equal(t, []string{"MISLEADING.CLAIM"}, violation.PolicyCodes)
	require.GreaterOrEqual(t, violation.Priority, 90)

	ranker.result = scoresFor([]string{"MISLEADING.CLAIM"}, 0.5)
	require.Equal(t, ReasonGrayZone, c.Run(context.Background(), p, rescan(nil)).Reason)

	// 行业禁用自动通过、首次送审保护期与图片未确认不适用于回扫。
	clear := &Cascade{Router: coveredRouter(0.1), Ranker: &fakeRanker{}, Lookup: &fakeLookup{}}
	result := clear.Run(context.Background(), p, rescan(func(in *Input) {
		in.Snapshot.Industry = "FINANCIAL"
		in.Snapshot.Media = []event.ReviewMedia{{MediaID: 1, SHA256: "unseen"}}
	}))
	require.Equal(t, OutcomeApprove, result.Outcome)
	require.Equal(t, ReasonRescanClear, result.Reason)
	require.Empty(t, stage(result, StageFingerprint).Stage, "回扫不复用指纹")

	forced := clear.Run(context.Background(), p, rescan(func(in *Input) { in.Snapshot.Texts["body"] = "Guaranteed results" }))
	require.Equal(t, OutcomeHuman, forced.Outcome)
	require.Equal(t, ReasonForcedRule, forced.Reason)

	hard := clear.Run(context.Background(), p, rescan(func(in *Input) { in.Snapshot.LandingURL = "https://x.scam-giveaway.com" }))
	require.Equal(t, OutcomeReject, hard.Outcome)
	require.Equal(t, []string{"LANDING.DOMAIN"}, hard.PolicyCodes)
}

// RVW-011：回扫的精排失败同样转人审，不放宽为未发现违规。
func TestRescanRankerFailureEscalates(t *testing.T) {
	p := loadPolicy(t)
	in := baseInput()
	in.Purpose = event.ReviewPurposeRescan
	c := &Cascade{Ranker: &fakeRanker{err: ErrUnavailable}}
	result := c.Run(context.Background(), p, in)
	require.Equal(t, OutcomeHuman, result.Outcome)
	require.Equal(t, ReasonRankerUnavailable, result.Reason)
}
