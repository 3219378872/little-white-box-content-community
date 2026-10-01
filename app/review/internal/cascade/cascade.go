// Package cascade 实现机审级联：指纹复用 → 硬规则 → 召回（Router）→ 精排（Ranker）→ 决策
// （RVW-010～RVW-018，DES-review-platform「机审级联」）。
//
// 工程骨架参考 TikTok Filter-And-Refine 的 Router → Ranker 级联。广告先审后投，召回漏召会直接
// 变成漏放，所以召回只决定精排范围，不单独构成通过依据；任何降级都不放宽自动通过条件。
package cascade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"esx/app/review/internal/policy"
	"esx/app/review/internal/snapshot"
	"esx/pkg/adpolicy"
	"esx/pkg/event"
)

// 阶段名。
const (
	StageFingerprint = "fingerprint"
	StageRules       = "rules"
	StageRouter      = "router"
	StageRanker      = "ranker"
	StageDecision    = "decision"
)

// 处置。
const (
	OutcomeApprove = "approve"
	OutcomeReject  = "reject"
	OutcomeHuman   = "human"
)

// 降级与转人审原因（阶段记录与指标 label）。
const (
	ReasonRouterUnavailable = "router-unavailable"
	ReasonRouterNoCoverage  = "router-no-coverage"
	ReasonImageUnconfirmed  = "image-unconfirmed"
	ReasonRankerTimeout     = "ranker-timeout"
	ReasonRankerUnavailable = "ranker-unavailable"
	ReasonRankerInvalid     = "ranker-invalid"
	ReasonRulesUnavailable  = "rules-unavailable"
	ReasonProtection        = "first-submission-protection"
	ReasonIndustry          = "industry-no-auto-pass"
	ReasonGrayZone          = "gray-zone"
	ReasonForcedRule        = "forced-human-rule"
	ReasonQualification     = "qualification-review"
	ReasonPurpose           = "purpose-requires-human"
	ReasonRescanViolation   = "rescan-violation"
	ReasonRescanClear       = "rescan-clear"
)

// RankerTimeout 与 DES 一致：2 秒，worker 异步执行，不在用户请求路径上。
const RankerTimeout = 2 * time.Second

// ErrUnavailable 表示组件不可用（区别于超时与无效输出）。
var ErrUnavailable = errors.New("cascade: component unavailable")

// RouteInput 是召回输入。
type RouteInput struct {
	Market   string
	Language string
	Text     string
	Issues   []string
}

// RouteResult 是每个 issue 的最高相似度与命中种子。
type RouteResult struct {
	Version    string
	Similarity map[string]float64
	SeedIDs    map[string][]int64
	// Covered=false 表示该市场没有生效种子，召回无法给出放行依据
	Covered bool
}

// Router 是低成本、高召回的初筛。
type Router interface {
	Route(ctx context.Context, in RouteInput) (RouteResult, error)
}

// RankInput 是精排输入；广告文案只作为数据字段传递（prompt 注入防护）。
type RankInput struct {
	TaskID   int64
	Market   string
	Language string
	Text     string
	Media    []string
	Issues   []string
}

// IssueScore 是单个 issue 的 Yes/No 概率。
type IssueScore struct {
	Issue string
	PYes  float64
	PNo   float64
}

// RankResult 是精排输出。
type RankResult struct {
	ModelVersion string
	Scores       []IssueScore
}

// Ranker 只对候选 issue 输出违规概率。
type Ranker interface {
	Score(ctx context.Context, in RankInput) (RankResult, error)
}

// Lookup 是级联需要的只读存储。
type Lookup interface {
	LookupVerdict(ctx context.Context, hash, market, policyVersion string) (verdict string, codes []string, source string, found bool, err error)
	ApprovedMedia(ctx context.Context, hashes []string) (map[string]bool, error)
}

// Input 是一次机审的任务信息。
type Input struct {
	TaskID        int64
	BizType       string
	Purpose       string
	SnapshotHash  string
	Snapshot      event.ReviewSnapshot
	SubmissionSeq int
}

// StageRecord 是一个阶段的版本、耗时与输出（RVW-017）。
type StageRecord struct {
	Stage            string
	ComponentVersion string
	Outcome          string
	Reason           string
	Output           string
	LatencyMs        int64
}

// Result 是级联结论。
type Result struct {
	Outcome     string
	PolicyCodes []string
	Reason      string
	Priority    int
	Stages      []StageRecord
}

// Cascade 组合级联组件。Router 与 Ranker 可为 nil，视为不可用。
type Cascade struct {
	Router RouterWithVersion
	Ranker Ranker
	Lookup Lookup
	Clock  func() time.Time
}

// RouterWithVersion 让不可用的召回也能在阶段记录中写明版本。
type RouterWithVersion interface {
	Router
	Version() string
}

// Run 按政策版本执行级联。shadow 运行使用同一流程，由调用方决定是否落结论（RVW-016）。
func (c *Cascade) Run(ctx context.Context, p *policy.Policy, in Input) Result {
	run := &runner{c: c, p: p, in: in}
	return run.execute(ctx)
}

type runner struct {
	c      *Cascade
	p      *policy.Policy
	in     Input
	stages []StageRecord
}

func (r *runner) now() time.Time {
	if r.c.Clock != nil {
		return r.c.Clock()
	}
	return time.Now()
}

func (r *runner) record(stage, version, outcome, reason string, output any, started time.Time) {
	raw, err := json.Marshal(output)
	if err != nil {
		raw = []byte(`{}`)
	}
	r.stages = append(r.stages, StageRecord{
		Stage: stage, ComponentVersion: version, Outcome: outcome, Reason: reason,
		Output: string(raw), LatencyMs: r.now().Sub(started).Milliseconds(),
	})
}

func (r *runner) finish(outcome string, codes []string, reason string, priority int) Result {
	r.record(StageDecision, "decision@"+r.p.Version, outcome, reason,
		map[string]any{"policyCodes": codes, "priority": priority}, r.now())
	return Result{Outcome: outcome, PolicyCodes: codes, Reason: reason, Priority: priority, Stages: r.stages}
}

func (r *runner) execute(ctx context.Context) Result {
	snap := r.in.Snapshot
	disposition := adpolicy.DispositionOf(snap.Market, snap.Industry)

	// 质检、申诉与举报由人工处理；资质对象由具备资质审核权限的审核员处理（RVW-023）。
	if r.in.Purpose == event.ReviewPurposeAppeal || r.in.Purpose == event.ReviewPurposeReport || r.in.Purpose == event.ReviewPurposeQA {
		return r.finish(OutcomeHuman, nil, ReasonPurpose, 50)
	}

	// S1 指纹复用（RVW-015）：只在哈希、市场与政策版本一致时命中；通过结论只来自人工或质检。
	protected := r.in.SubmissionSeq > 0 && r.in.SubmissionSeq <= r.p.ProtectionSubmissions
	if r.in.Purpose == event.ReviewPurposeInitial {
		started := r.now()
		verdict, codes, source, found, err := r.lookupVerdict(ctx)
		switch {
		case err != nil:
			r.record(StageFingerprint, "verdict-cache@"+r.p.Version, "miss", "lookup-error", map[string]any{}, started)
		case found && verdict == event.ReviewVerdictReject:
			r.record(StageFingerprint, "verdict-cache@"+r.p.Version, "hit", "", map[string]any{"verdict": verdict, "source": source}, started)
			return r.finish(OutcomeReject, codes, "fingerprint-reject", 0)
		case found && verdict == event.ReviewVerdictApprove && !protected && r.in.BizType == event.ReviewBizAdCreative &&
			(source == event.ReviewSourceHuman || source == event.ReviewSourceQA):
			r.record(StageFingerprint, "verdict-cache@"+r.p.Version, "hit", "", map[string]any{"verdict": verdict, "source": source}, started)
			return r.finish(OutcomeApprove, nil, "fingerprint-approve", 0)
		default:
			reason := ""
			if found {
				reason = ReasonProtection
			}
			r.record(StageFingerprint, "verdict-cache@"+r.p.Version, "miss", reason, map[string]any{"found": found}, started)
		}
	}

	if r.in.BizType == event.ReviewBizAdvertiserQualification {
		return r.finish(OutcomeHuman, nil, ReasonQualification, 40)
	}

	// S2 硬规则：行业处置、落地页结构与域名、素材黑样本、关键词。
	started := r.now()
	ruleHits, forced := r.applyRules(disposition)
	rejectCodes := ruleCodes(ruleHits, policy.ActionReject)
	if len(rejectCodes) > 0 {
		r.record(StageRules, "rules@"+r.p.Version, "reject", "", ruleHits, started)
		return r.finish(OutcomeReject, rejectCodes, "hard-rule", 0)
	}
	if forced {
		r.record(StageRules, "rules@"+r.p.Version, "human", ReasonForcedRule, ruleHits, started)
	} else {
		r.record(StageRules, "rules@"+r.p.Version, "pass", "", ruleHits, started)
	}

	// S3 Router：召回不可用或无覆盖时全部 issue 进入精排（RVW-011）。
	candidates, routerReason := r.route(ctx)
	imageUnconfirmed := r.imageUnconfirmed(ctx)

	// S4 Ranker：只对候选 issue 精排；失败转人审。
	var scores []IssueScore
	if len(candidates) > 0 {
		var reason string
		scores, reason = r.rank(ctx, candidates)
		if reason != "" {
			return r.finish(OutcomeHuman, nil, reason, 60)
		}
	} else {
		r.record(StageRanker, "ranker:skipped", "pass", "no-candidates", map[string]any{}, r.now())
	}

	// S5 决策（RVW-012、RVW-013）。
	maxYes := 0.0
	var autoReject, violations []string
	allBelowPass := true
	for _, score := range scores {
		threshold := r.p.ThresholdFor(score.Issue, snap.Market)
		maxYes = math.Max(maxYes, score.PYes)
		if score.PYes >= threshold.Reject {
			violations = append(violations, score.Issue)
			if threshold.AutoReject {
				autoReject = append(autoReject, score.Issue)
			}
		}
		if score.PYes >= threshold.Pass {
			allBelowPass = false
		}
	}
	priority := int(maxYes * 100)
	if r.in.Purpose == event.ReviewPurposeRescan {
		return r.rescanDecision(violations, forced, allBelowPass, ruleHits, priority)
	}
	if len(autoReject) > 0 {
		sort.Strings(autoReject)
		return r.finish(OutcomeReject, autoReject, "ranker-reject", priority)
	}
	switch {
	case forced:
		return r.finish(OutcomeHuman, ruleCodes(ruleHits, policy.ActionHuman), ReasonForcedRule, max(priority, 70))
	case !allBelowPass:
		return r.finish(OutcomeHuman, nil, ReasonGrayZone, priority)
	case !disposition.AutoPassAllowed:
		return r.finish(OutcomeHuman, nil, ReasonIndustry, priority)
	case protected:
		return r.finish(OutcomeHuman, nil, ReasonProtection, priority)
	case imageUnconfirmed:
		return r.finish(OutcomeHuman, nil, ReasonImageUnconfirmed, priority)
	}
	reason := "auto-pass"
	if routerReason != "" {
		reason = "auto-pass;" + routerReason
	}
	return r.finish(OutcomeApprove, nil, reason, 0)
}

// rescanDecision 是回扫的处置（ADS-031）。回扫对象已经过审在投，结论不是放行而是是否违规：
//   - 任一候选 issue 的分数达到拒绝阈值即判定违规（不论该 issue 是否允许自动拒绝，因为结果是暂停
//     并转人审而非终审拒绝），硬规则命中在 S2 已以拒绝返回；
//   - 强制人审规则或灰区分数转人审核实，不暂停投放；
//   - 其余视为未发现违规；行业、首次送审保护期与图片确认是授予自动通过的条件，不适用于回扫。
func (r *runner) rescanDecision(violations []string, forced, allBelowPass bool, ruleHits []ruleHit, priority int) Result {
	switch {
	case len(violations) > 0:
		sort.Strings(violations)
		return r.finish(OutcomeReject, violations, ReasonRescanViolation, max(priority, 90))
	case forced:
		return r.finish(OutcomeHuman, ruleCodes(ruleHits, policy.ActionHuman), ReasonForcedRule, max(priority, 70))
	case !allBelowPass:
		return r.finish(OutcomeHuman, nil, ReasonGrayZone, priority)
	}
	return r.finish(OutcomeApprove, nil, ReasonRescanClear, 0)
}

func (r *runner) lookupVerdict(ctx context.Context) (string, []string, string, bool, error) {
	if r.c.Lookup == nil {
		return "", nil, "", false, nil
	}
	return r.c.Lookup.LookupVerdict(ctx, r.in.SnapshotHash, r.in.Snapshot.Market, r.p.Version)
}

type ruleHit struct {
	Rule   string `json:"rule"`
	Code   string `json:"code"`
	Action string `json:"action"`
	Term   string `json:"term,omitempty"`
}

func (r *runner) applyRules(disposition adpolicy.Disposition) ([]ruleHit, bool) {
	snap := r.in.Snapshot
	hits := []ruleHit{}
	if !adpolicy.IsMarket(snap.Market) || (snap.Industry != "" && !adpolicy.IsIndustry(snap.Industry)) {
		hits = append(hits, ruleHit{Rule: "market-industry", Code: "FORMAT.FUNCTIONALITY", Action: policy.ActionReject})
	}
	if disposition.Forbidden {
		hits = append(hits, ruleHit{Rule: "industry", Code: disposition.PolicyCode, Action: policy.ActionReject})
	}
	if snap.LandingURL != "" {
		domain, ok := adpolicy.LandingURLValid(snap.LandingURL)
		switch {
		case !ok:
			hits = append(hits, ruleHit{Rule: "landing-url", Code: adpolicy.CodeLandingURL, Action: policy.ActionReject})
		case blockedDomain(domain, r.p.BlockedDomains):
			hits = append(hits, ruleHit{Rule: "landing-domain", Code: adpolicy.CodeLandingDomain, Action: policy.ActionReject, Term: domain})
		}
	}
	for _, media := range snap.Media {
		if code, ok := r.p.BlockedMedia[media.SHA256]; ok {
			hits = append(hits, ruleHit{Rule: "media-sha256", Code: code, Action: policy.ActionReject})
		}
	}
	// 规则自身规范化，不依赖调用方先冻结快照。
	text := strings.ToLower(snapshot.NormalizeText(joinTexts(snap.Texts)))
	for _, rule := range r.p.Keywords {
		for _, terms := range rule.Terms {
			for _, term := range terms {
				if strings.Contains(text, term) {
					hits = append(hits, ruleHit{Rule: "keyword", Code: rule.Code, Action: rule.Action, Term: term})
				}
			}
		}
	}
	forced := slices.ContainsFunc(hits, func(h ruleHit) bool { return h.Action == policy.ActionHuman })
	return hits, forced
}

func (r *runner) route(ctx context.Context) ([]string, string) {
	started := r.now()
	issues := slices.Clone(r.p.Issues)
	if r.c.Router == nil {
		r.record(StageRouter, "router:none", "degraded", ReasonRouterUnavailable, map[string]any{"candidates": issues}, started)
		return issues, ReasonRouterUnavailable
	}
	result, err := r.c.Router.Route(ctx, RouteInput{
		Market: r.in.Snapshot.Market, Language: r.in.Snapshot.Language,
		Text: joinTexts(r.in.Snapshot.Texts), Issues: issues,
	})
	if err != nil {
		r.record(StageRouter, r.c.Router.Version(), "degraded", ReasonRouterUnavailable,
			map[string]any{"candidates": issues, "error": errorClass(err)}, started)
		return issues, ReasonRouterUnavailable
	}
	if !result.Covered {
		r.record(StageRouter, result.Version, "degraded", ReasonRouterNoCoverage,
			map[string]any{"candidates": issues}, started)
		return issues, ReasonRouterNoCoverage
	}
	var candidates []string
	for _, issue := range issues {
		if result.Similarity[issue] >= r.p.ThresholdFor(issue, r.in.Snapshot.Market).Route {
			candidates = append(candidates, issue)
		}
	}
	r.record(StageRouter, result.Version, "pass", "", map[string]any{
		"candidates": candidates, "similarity": result.Similarity, "seeds": result.SeedIDs,
	}, started)
	return candidates, ""
}

// imageUnconfirmed：图片向量召回是占位接口；未在人工通过快照中出现过的图片不满足自动通过条件。
func (r *runner) imageUnconfirmed(ctx context.Context) bool {
	var hashes []string
	for _, media := range r.in.Snapshot.Media {
		hashes = append(hashes, media.SHA256)
	}
	if len(hashes) == 0 {
		return false
	}
	if r.c.Lookup == nil {
		return true
	}
	known, err := r.c.Lookup.ApprovedMedia(ctx, hashes)
	if err != nil {
		return true
	}
	for _, hash := range hashes {
		if !known[hash] {
			return true
		}
	}
	return false
}

func (r *runner) rank(ctx context.Context, candidates []string) ([]IssueScore, string) {
	started := r.now()
	if r.c.Ranker == nil {
		r.record(StageRanker, "ranker:none", "degraded", ReasonRankerUnavailable, map[string]any{"issues": candidates}, started)
		return nil, ReasonRankerUnavailable
	}
	var media []string
	for _, m := range r.in.Snapshot.Media {
		media = append(media, m.SHA256)
	}
	rankCtx, cancel := context.WithTimeout(ctx, RankerTimeout)
	defer cancel()
	result, err := r.c.Ranker.Score(rankCtx, RankInput{
		TaskID: r.in.TaskID, Market: r.in.Snapshot.Market, Language: r.in.Snapshot.Language,
		Text: joinTexts(r.in.Snapshot.Texts), Media: media, Issues: candidates,
	})
	if err != nil {
		reason := ReasonRankerUnavailable
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(rankCtx.Err(), context.DeadlineExceeded) {
			reason = ReasonRankerTimeout
		}
		r.record(StageRanker, "ranker:error", "degraded", reason, map[string]any{"issues": candidates, "error": errorClass(err)}, started)
		return nil, reason
	}
	if err := ValidateRank(candidates, result); err != nil {
		r.record(StageRanker, nonEmpty(result.ModelVersion, "ranker:invalid"), "degraded", ReasonRankerInvalid,
			map[string]any{"issues": candidates, "error": err.Error()}, started)
		return nil, ReasonRankerInvalid
	}
	r.record(StageRanker, result.ModelVersion, "pass", "", map[string]any{"scores": result.Scores}, started)
	return result.Scores, ""
}

// ValidateRank 校验每个请求 issue 都有分数、概率在 0～1 且两者之和接近 1、版本非空。
func ValidateRank(issues []string, result RankResult) error {
	if strings.TrimSpace(result.ModelVersion) == "" {
		return fmt.Errorf("ranker: empty model version")
	}
	byIssue := map[string]IssueScore{}
	for _, score := range result.Scores {
		if _, dup := byIssue[score.Issue]; dup {
			return fmt.Errorf("ranker: duplicate score for %s", score.Issue)
		}
		if math.IsNaN(score.PYes) || math.IsNaN(score.PNo) || score.PYes < 0 || score.PYes > 1 || score.PNo < 0 || score.PNo > 1 {
			return fmt.Errorf("ranker: probability out of range for %s", score.Issue)
		}
		if math.Abs(score.PYes+score.PNo-1) > 0.05 {
			return fmt.Errorf("ranker: probabilities for %s do not sum to 1", score.Issue)
		}
		byIssue[score.Issue] = score
	}
	for _, issue := range issues {
		if _, ok := byIssue[issue]; !ok {
			return fmt.Errorf("ranker: missing score for %s", issue)
		}
	}
	if len(byIssue) != len(issues) {
		return fmt.Errorf("ranker: unexpected issues in response")
	}
	return nil
}

func ruleCodes(hits []ruleHit, action string) []string {
	var codes []string
	for _, hit := range hits {
		if hit.Action == action && !slices.Contains(codes, hit.Code) {
			codes = append(codes, hit.Code)
		}
	}
	sort.Strings(codes)
	return codes
}

func blockedDomain(domain string, blocked []string) bool {
	for _, b := range blocked {
		if domain == b || strings.HasSuffix(domain, "."+b) {
			return true
		}
	}
	return false
}

// JoinTexts 以稳定顺序拼接文案，供规则、召回与精排共用。
func joinTexts(texts map[string]string) string {
	keys := make([]string, 0, len(texts))
	for key := range texts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if texts[key] != "" {
			parts = append(parts, texts[key])
		}
	}
	return strings.Join(parts, "\n")
}

// JoinTexts 导出给种子提名等调用方。
func JoinTexts(texts map[string]string) string { return joinTexts(texts) }

func errorClass(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrUnavailable):
		return "unavailable"
	default:
		return "error"
	}
}

func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
