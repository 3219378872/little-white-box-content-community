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

// Request 按结果统计广告槽位请求（served/empty/cached/no-identity）。
func Request(result string) { requests.Inc(result) }

// Slots 按市场统计下发的广告槽位数。
func Slots(market string, n int) { slots.Add(float64(n), market) }

// Capped 统计因频控未下发的广告数。
func Capped(n int) { capped.Add(float64(n)) }

// Degraded 按组件与原因统计降级次数。
func Degraded(component, reason string) { degraded.Inc(component, reason) }

// Applied 按对象类型统计审核结论的应用结果（applied/stale）。
func Applied(bizType, result string) { applied.Inc(bizType, result) }

// Reconcile 统计送审对账的结果（confirmed/repaired/error）。
func Reconcile(bizType, outcome string) { repairs.Inc(bizType, outcome) }

// Report 统计举报（counted/duplicate）。
func Report(outcome string) { reports.Inc(outcome) }

// Appeal 统计申诉次数。
func Appeal() { appeals.Inc() }

// Rescan 统计复扫处理结果。
func Rescan(outcome string) { rescans.Inc(outcome) }

// Propagation 记录审核结论到投放索引生效的耗时，用于验证下线在时限内生效（ADS-032）；
// 缺少结论时间时不记录样本（而非记 0），避免把未知耗时混进直方图。
func Propagation(action string, decidedAtMs int64, now time.Time) {
	if decidedAtMs > 0 {
		propagation.ObserveFloat(metric.EventLagSeconds(decidedAtMs, now), action)
	}
}
