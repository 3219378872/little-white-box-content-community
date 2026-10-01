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
	shadowRuns = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "review", Name: "shadow_runs_total",
		Help: "Shadow policy runs by outcome and agreement", Labels: []string{"outcome", "agrees"},
	})
)

func Ingested(bizType, result string) { ingested.Inc(bizType, result) }

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

func Stage(stage string, latencyMs int64) { stageLatency.ObserveFloat(float64(latencyMs)/1000, stage) }

func Degraded(stage, reason string) { degraded.Inc(stage, reason) }

func Fingerprint(outcome string) { fingerprint.Inc(outcome) }

func Backlog(pending int64, oldestSubmitMs int64, now time.Time) {
	humanBacklog.Set(float64(pending))
	age := 0.0
	if pending > 0 && oldestSubmitMs > 0 {
		age = max(0, now.Sub(time.UnixMilli(oldestSubmitMs)).Seconds())
	}
	oldestAge.Set(age)
}

func QADisagreement(originalSource string) { qaDisagreements.Inc(originalSource) }

func AppealOverturn() { appealOverturns.Inc() }

func AttemptsExceeded() { attemptsExceeded.Inc() }

func Shadow(outcome string, agrees bool) {
	label := "false"
	if agrees {
		label = "true"
	}
	shadowRuns.Inc(outcome, label)
}
