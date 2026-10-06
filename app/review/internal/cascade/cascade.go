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

// runner 保存单次级联的上下文与已完成阶段记录；每次 Run 新建，互不共享。
type runner struct {
	c      *Cascade
	p      *policy.Policy
	in     Input
	stages []StageRecord
}

// now 允许测试注入时钟，使阶段耗时可断言。
func (r *runner) now() time.Time {
	if r.c.Clock != nil {
		return r.c.Clock()
	}
	return time.Now()
}

// record 追加一条阶段记录（RVW-017）；输出无法序列化时写空对象，不让审计记录缺失。
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

// finish 写入决策阶段记录并产出最终结论，所有返回路径都经过这里以保证阶段记录完整。
func (r *runner) finish(outcome string, codes []string, reason string, priority int) Result {
	r.record(StageDecision, "decision@"+r.p.Version, outcome, reason,
		map[string]any{"policyCodes": codes, "priority": priority}, r.now())
	return Result{Outcome: outcome, PolicyCodes: codes, Reason: reason, Priority: priority, Stages: r.stages}
}

// execute 按 S1→S5 顺序执行级联；任一阶段得出终局结论即提前返回，其余信号汇总到 S5 决策。
func (r *runner) execute(ctx context.Context) Result {
	snap := r.in.Snapshot
	disposition := adpolicy.DispositionOf(snap.Market, snap.Industry)

	// 质检、申诉与举报由人工处理；资质对象由具备资质审核权限的审核员处理（RVW-023）。
	if r.in.Purpose == event.ReviewPurposeAppeal || r.in.Purpose == event.ReviewPurposeReport || r.in.Purpose == event.ReviewPurposeQA {
		return r.finish(OutcomeHuman, nil, ReasonPurpose, 50)
	}

	// S1 指纹复用（RVW-015）。首次送审保护期内即使命中通过指纹也不自动放行。
	protected := r.in.SubmissionSeq > 0 && r.in.SubmissionSeq <= r.p.ProtectionSubmissions
	if r.in.Purpose == event.ReviewPurposeInitial {
		if result, done := r.fingerprintStage(ctx, protected); done {
			return result
		}
	}

	if r.in.BizType == event.ReviewBizAdvertiserQualification {
		return r.finish(OutcomeHuman, nil, ReasonQualification, 40)
	}

	// S2 硬规则：命中拒绝规则直接拒绝；强制人审规则只记下，留到 S5 与分数一起决策。
	ruleHits, forced, result, done := r.rulesStage(disposition)
	if done {
		return result
	}

	// S3 Router：召回不可用或无覆盖时全部 issue 进入精排（RVW-011）。
	candidates, routerReason := r.route(ctx)
	imageUnconfirmed := r.imageUnconfirmed(ctx)

	// S4 Ranker：只对候选 issue 精排；失败转人审。
	scores, result, done := r.rankStage(ctx, candidates)
	if done {
		return result
	}

	// S5 决策（RVW-012、RVW-013）。
	return r.decide(decisionSignals{
		disposition: disposition, protected: protected,
		ruleHits: ruleHits, forced: forced,
		routerReason: routerReason, imageUnconfirmed: imageUnconfirmed,
		scores: summarizeScores(r.p, snap.Market, scores),
	})
}

// fingerprintStage 只在哈希、市场与政策版本一致时复用历史结论；拒绝可直接复用，
// 通过结论只接受人工或质检来源的广告创意，且不在首次送审保护期内。done=false 表示继续级联。
func (r *runner) fingerprintStage(ctx context.Context, protected bool) (Result, bool) {
	started := r.now()
	version := "verdict-cache@" + r.p.Version
	verdict, codes, source, found, err := r.lookupVerdict(ctx)
	switch {
	case err != nil:
		// 查询失败只当作未命中，不阻断后续机审。
		r.record(StageFingerprint, version, "miss", "lookup-error", map[string]any{}, started)
	case found && verdict == event.ReviewVerdictReject:
		r.record(StageFingerprint, version, "hit", "", map[string]any{"verdict": verdict, "source": source}, started)
		return r.finish(OutcomeReject, codes, "fingerprint-reject", 0), true
	case found && verdict == event.ReviewVerdictApprove && !protected && r.in.BizType == event.ReviewBizAdCreative &&
		(source == event.ReviewSourceHuman || source == event.ReviewSourceQA):
		r.record(StageFingerprint, version, "hit", "", map[string]any{"verdict": verdict, "source": source}, started)
		return r.finish(OutcomeApprove, nil, "fingerprint-approve", 0), true
	default:
		// 命中但不满足复用条件时记下保护期原因，便于解释为何没有复用。
		reason := ""
		if found {
			reason = ReasonProtection
		}
		r.record(StageFingerprint, version, "miss", reason, map[string]any{"found": found}, started)
	}
	return Result{}, false
}

// rulesStage 执行行业处置、落地页结构与域名、素材黑样本、关键词规则。
// 返回全部命中与是否命中强制人审规则；命中拒绝规则时 done=true 并给出拒绝结论。
func (r *runner) rulesStage(disposition adpolicy.Disposition) ([]ruleHit, bool, Result, bool) {
	started := r.now()
	version := "rules@" + r.p.Version
	ruleHits, forced := r.applyRules(disposition)
	if rejectCodes := ruleCodes(ruleHits, policy.ActionReject); len(rejectCodes) > 0 {
		r.record(StageRules, version, "reject", "", ruleHits, started)
		return ruleHits, forced, r.finish(OutcomeReject, rejectCodes, "hard-rule", 0), true
	}
	if forced {
		r.record(StageRules, version, "human", ReasonForcedRule, ruleHits, started)
	} else {
		r.record(StageRules, version, "pass", "", ruleHits, started)
	}
	return ruleHits, forced, Result{}, false
}

// rankStage 对召回候选精排；没有候选时记录跳过。精排超时、不可用或输出无效时 done=true 转人审。
func (r *runner) rankStage(ctx context.Context, candidates []string) ([]IssueScore, Result, bool) {
	if len(candidates) == 0 {
		r.record(StageRanker, "ranker:skipped", "pass", "no-candidates", map[string]any{}, r.now())
		return nil, Result{}, false
	}
	scores, reason := r.rank(ctx, candidates)
	if reason != "" {
		return nil, r.finish(OutcomeHuman, nil, reason, 60), true
	}
	return scores, Result{}, false
}

// scoreSummary 是精排分数对照政策阈值后的汇总。
type scoreSummary struct {
	// priority 取最高违规概率的百分数，用于人审排序。
	priority int
	// violations 是达到拒绝阈值的 issue；autoReject 是其中政策允许自动拒绝的部分。
	violations []string
	autoReject []string
	// allBelowPass 表示全部 issue 都低于放行阈值，即不处在灰区。
	allBelowPass bool
}

// summarizeScores 按市场阈值把每个 issue 的违规概率归入拒绝、灰区或放行。
func summarizeScores(p *policy.Policy, market string, scores []IssueScore) scoreSummary {
	maxYes := 0.0
	summary := scoreSummary{allBelowPass: true}
	for _, score := range scores {
		threshold := p.ThresholdFor(score.Issue, market)
		maxYes = math.Max(maxYes, score.PYes)
		if score.PYes >= threshold.Reject {
			summary.violations = append(summary.violations, score.Issue)
			if threshold.AutoReject {
				summary.autoReject = append(summary.autoReject, score.Issue)
			}
		}
		if score.PYes >= threshold.Pass {
			summary.allBelowPass = false
		}
	}
	summary.priority = int(maxYes * 100)
	return summary
}

// decisionSignals 汇总 S1–S4 留给 S5 决策的信号。
type decisionSignals struct {
	disposition      adpolicy.Disposition
	protected        bool
	ruleHits         []ruleHit
	forced           bool
	routerReason     string
	imageUnconfirmed bool
	scores           scoreSummary
}

// decide 是 S5：可自动拒绝的违规直接拒绝；否则任何一个“不能自动通过”的信号都转人审，
// 只有所有信号都放行时才自动通过（召回降级只写进原因，不放宽通过条件）。回扫走独立处置。
func (r *runner) decide(in decisionSignals) Result {
	priority := in.scores.priority
	if r.in.Purpose == event.ReviewPurposeRescan {
		return r.rescanDecision(in.scores.violations, in.forced, in.scores.allBelowPass, in.ruleHits, priority)
	}
	if len(in.scores.autoReject) > 0 {
		autoReject := in.scores.autoReject
		sort.Strings(autoReject)
		return r.finish(OutcomeReject, autoReject, "ranker-reject", priority)
	}
	// 转人审原因按优先顺序判断：强制规则 > 灰区分数 > 行业禁止自动通过 > 保护期 > 图片未确认。
	switch {
	case in.forced:
		return r.finish(OutcomeHuman, ruleCodes(in.ruleHits, policy.ActionHuman), ReasonForcedRule, max(priority, 70))
	case !in.scores.allBelowPass:
		return r.finish(OutcomeHuman, nil, ReasonGrayZone, priority)
	case !in.disposition.AutoPassAllowed:
		return r.finish(OutcomeHuman, nil, ReasonIndustry, priority)
	case in.protected:
		return r.finish(OutcomeHuman, nil, ReasonProtection, priority)
	case in.imageUnconfirmed:
		return r.finish(OutcomeHuman, nil, ReasonImageUnconfirmed, priority)
	}
	reason := "auto-pass"
	if in.routerReason != "" {
		reason = "auto-pass;" + in.routerReason
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

// lookupVerdict 在未配置存储时视为未命中。
func (r *runner) lookupVerdict(ctx context.Context) (string, []string, string, bool, error) {
	if r.c.Lookup == nil {
		return "", nil, "", false, nil
	}
	return r.c.Lookup.LookupVerdict(ctx, r.in.SnapshotHash, r.in.Snapshot.Market, r.p.Version)
}

// ruleHit 是一条规则命中，作为规则阶段的输出写入阶段记录。
type ruleHit struct {
	Rule   string `json:"rule"`
	Code   string `json:"code"`
	Action string `json:"action"`
	Term   string `json:"term,omitempty"`
}

// applyRules 逐类收集硬规则命中，不在首个命中处停止，以便阶段记录完整列出全部违规。
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

// route 返回需要精排的候选 issue 与降级原因。召回不可用或市场无种子覆盖时退化为全部 issue，
// 宁可多精排也不漏召（RVW-011）。
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
	// 只保留相似度达到市场召回阈值的 issue。
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

// rank 在 RankerTimeout 内为候选 issue 打分，返回分数或降级原因（超时、不可用、输出无效）。
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
	// 模型输出必须逐个覆盖候选 issue 且概率合法，否则不能作为决策依据。
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

// ruleCodes 收集指定动作的命中代码，去重并排序，保证同一输入的输出稳定。
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

// blockedDomain 判断域名本身或其任一上级域名是否在封禁名单中。
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

// errorClass 把依赖错误归为 timeout/unavailable/error，用作降级原因标签。
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

// nonEmpty 在值为空时返回兜底值。
func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
