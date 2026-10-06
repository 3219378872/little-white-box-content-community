// Package metrics 定义审核平台指标（前缀 esx_review_，RVW-060）。机审自动化率由结论计数推导；
// 送审到结论的耗时按机审与人审分开观察，P95 对照 24 小时目标如实报告（RVW-061）。
package metrics

import (
	"time"

	metric "esx/pkg/metrics"
)

var (
	ingested = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "ingested_total",
		Help: "Review submissions by intake result", Labels: []string{"biz_type", "result"},
	})
	decisions = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "decisions_total",
		Help: "Review decisions by source, verdict and purpose", Labels: []string{"source", "verdict", "purpose"},
	})
	submitToDecision = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "submit_to_decision_seconds",
		Help:    "Elapsed time from submission to first decision",
		Labels:  []string{"path"},
		Buckets: []float64{1, 5, 30, 60, 300, 900, 3600, 4 * 3600, 12 * 3600, 24 * 3600, 48 * 3600, 7 * 24 * 3600},
	})
	stageLatency = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "stage_seconds",
		Help: "Machine review stage latency", Labels: []string{"stage"},
		Buckets: []float64{0.001, 0.005, 0.02, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	})
	degraded = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "degraded_total",
		Help: "Machine review component degradations", Labels: []string{"stage", "reason"},
	})
	fingerprint = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "fingerprint_lookups_total",
		Help: "Fingerprint reuse lookups by outcome", Labels: []string{"outcome"},
	})
	humanBacklog = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "human_backlog",
		Help: "Human review tasks waiting or held", Labels: []string{},
	})
	oldestAge = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "oldest_human_task_age_seconds",
		Help: "Age of the oldest human review task", Labels: []string{},
	})
	qaDisagreements = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "qa_disagreements_total",
		Help: "QA decisions that differ from the sampled decision", Labels: []string{"original_source"},
	})
	appealOverturns = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "appeal_overturns_total",
		Help: "Appeal decisions that changed the original verdict", Labels: []string{},
	})
	attemptsExceeded = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "claim_attempts_exceeded_total",
		Help: "Human tasks claimed more than the attempt limit", Labels: []string{},
	})
	rescanPauses = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "rescan_pauses_total",
		Help: "Rescans judged as violations that paused serving pending human review", Labels: []string{},
	})
	shadowRuns = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "shadow_runs_total",
		Help: "Shadow policy runs by outcome and agreement", Labels: []string{"outcome", "agrees"},
	})
)

// Ingested 按对象类型统计送审接入结果。
func Ingested(bizType, result string) { ingested.Inc(bizType, result) }

// Decision 统计审核结论，并按机审/人审路径记录送审到出结论的耗时。
func Decision(source, verdict, purpose string, submittedAtMs int64, now time.Time) {
	decisions.Inc(source, verdict, purpose)
	path := "human"
	if source == "machine" {
		path = "machine"
	}
	if submittedAtMs > 0 {
		submitToDecision.ObserveFloat(max(0, now.Sub(time.UnixMilli(submittedAtMs)).Seconds()), path)
	}
}

// Stage 记录机审各阶段的耗时（毫秒转秒）。
func Stage(stage string, latencyMs int64) { stageLatency.ObserveFloat(float64(latencyMs)/1000, stage) }

// Degraded 按阶段与原因统计机审降级。
func Degraded(stage, reason string) { degraded.Inc(stage, reason) }

// Fingerprint 统计指纹阶段的命中结果。
func Fingerprint(outcome string) { fingerprint.Inc(outcome) }

// Backlog 更新人审积压量与最老任务的等待时长；没有积压时等待时长记为 0。
func Backlog(pending int64, oldestSubmitMs int64, now time.Time) {
	humanBacklog.Set(float64(pending))
	age := 0.0
	if pending > 0 && oldestSubmitMs > 0 {
		age = max(0, now.Sub(time.UnixMilli(oldestSubmitMs)).Seconds())
	}
	oldestAge.Set(age)
}

// QADisagreement 统计抽检与原结论不一致的次数，按原结论来源区分。
func QADisagreement(originalSource string) { qaDisagreements.Inc(originalSource) }

// AppealOverturn 统计申诉改判次数。
func AppealOverturn() { appealOverturns.Inc() }

// AttemptsExceeded 统计机审重试次数耗尽、转人审的任务。
func AttemptsExceeded() { attemptsExceeded.Inc() }

// RescanPaused 记录回扫判定违规并暂停投放（ADS-031）。
func RescanPaused() { rescanPauses.Inc() }

// Shadow 统计影子政策的运行结果，以及与生效政策结论是否一致。
func Shadow(outcome string, agrees bool) {
	label := "false"
	if agrees {
		label = "true"
	}
	shadowRuns.Inc(outcome, label)
}
