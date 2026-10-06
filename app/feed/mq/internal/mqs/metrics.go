package mqs

import (
	"time"

	metric "esx/pkg/metrics"
)

var (
	feedConsumerMessages = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "feed_consumer", Name: "messages_total",
		Help: "Feed fanout messages by processing outcome", Labels: []string{"outcome"},
	})
	feedConsumerEventLag = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "feed_consumer", Name: "event_lag_seconds",
		Help:    "Elapsed time from post event occurrence to feed fanout",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300, 900, 3600},
	})
)

// observeFeedLag 记录事件发生到 fanout 完成的延迟，用于观察关注流的新鲜度。
func observeFeedLag(eventTime int64, now time.Time) {
	feedConsumerEventLag.ObserveFloat(metric.EventLagSeconds(eventTime, now))
}
