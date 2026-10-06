package mqs

import (
	"time"

	metric "esx/pkg/metrics"
)

var (
	cleanupConsumerMessages = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "content_cleanup_consumer", Name: "messages_total",
		Help: "Content cleanup messages by processing outcome", Labels: []string{"outcome"},
	})
	countSyncConsumerMessages = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "content_count_sync_consumer", Name: "messages_total",
		Help: "Content count sync messages by processing outcome", Labels: []string{"outcome"},
	})
	countSyncEventLag = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "content_count_sync_consumer", Name: "event_lag_seconds",
		Help:    "Elapsed time from interaction occurrence to post/comment count sync",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300, 900, 3600},
	})
)

// observeCountSyncLag 记录行为发生到计数同步完成的延迟。
func observeCountSyncLag(eventTime int64, now time.Time) {
	countSyncEventLag.ObserveFloat(countSyncLagSeconds(eventTime, now))
}

// countSyncLagSeconds 计算事件延迟秒数；缺少事件时间或时钟回拨时记为 0，避免污染直方图。
func countSyncLagSeconds(eventTime int64, now time.Time) float64 {
	if eventTime <= 0 {
		return 0
	}
	lag := now.Sub(time.UnixMilli(eventTime)).Seconds()
	if lag < 0 {
		return 0
	}
	return lag
}
