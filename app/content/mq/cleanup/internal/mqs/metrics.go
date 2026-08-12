package mqs

import (
	"time"

	"github.com/zeromicro/go-zero/core/metric"
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

func observeCountSyncLag(eventTime int64, now time.Time) {
	countSyncEventLag.ObserveFloat(countSyncLagSeconds(eventTime, now))
}

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
