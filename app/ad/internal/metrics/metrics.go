// Package metrics 定义广告指标（前缀 esx_ads_，ADS-050）。点击率由 ClickHouse 聚合计算。
package metrics

import (
	"time"

	metric "esx/pkg/metrics"
)

var (
	requests = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "ads", Name: "slot_requests_total",
		Help: "Sponsored slot requests by result", Labels: []string{"result"},
	})
	slots = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "ads", Name: "slots_served_total",
		Help: "Sponsored slots delivered by market", Labels: []string{"market"},
	})
	capped = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "ads", Name: "frequency_capped_total",
		Help: "Candidates excluded by the daily frequency cap", Labels: []string{},
	})
	degraded = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "ads", Name: "degraded_total",
		Help: "Sponsored slot degradations by reason", Labels: []string{"component", "reason"},
	})
	propagation = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "ads", Name: "serving_propagation_seconds",
		Help:    "Delay from review decision to serving index change",
		Labels:  []string{"action"},
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
	})
	applied = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "ads", Name: "decisions_applied_total",
		Help: "Review decisions applied by business type and result", Labels: []string{"biz_type", "result"},
	})
	reports = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "ads", Name: "reports_total",
		Help: "User ad reports by outcome", Labels: []string{"outcome"},
	})
	appeals = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "ads", Name: "appeals_total",
		Help: "Advertiser appeals submitted", Labels: []string{},
	})
	rescans = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "ads", Name: "rescans_total",
		Help: "Serving ads handled per rescan generation by outcome", Labels: []string{"outcome"},
	})
	repairs = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "ads", Name: "review_reconcile_total",
		Help: "Review reconciliation calls by business type and outcome", Labels: []string{"biz_type", "outcome"},
	})
)

func Request(result string)             { requests.Inc(result) }
func Slots(market string, n int)        { slots.Add(float64(n), market) }
func Capped(n int)                      { capped.Add(float64(n)) }
func Degraded(component, reason string) { degraded.Inc(component, reason) }
func Applied(bizType, result string)    { applied.Inc(bizType, result) }
func Reconcile(bizType, outcome string) { repairs.Inc(bizType, outcome) }
func Report(outcome string)             { reports.Inc(outcome) }
func Appeal()                           { appeals.Inc() }
func Rescan(outcome string)             { rescans.Inc(outcome) }
func Propagation(action string, decidedAtMs int64, now time.Time) {
	if decidedAtMs > 0 {
		propagation.ObserveFloat(max(0, now.Sub(time.UnixMilli(decidedAtMs)).Seconds()), action)
	}
}
