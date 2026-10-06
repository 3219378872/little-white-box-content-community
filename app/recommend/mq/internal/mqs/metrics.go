package mqs

import (
	"time"

	metric "esx/pkg/metrics"
)

var (
	recommendConsumerMessages = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "recommend_feature_consumer", Name: "messages_total",
		Help:   "Recommendation feature messages by stream and processing outcome",
		Labels: []string{"stream", "outcome"},
	})
	recommendFeatureEventLag = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "recommend_feature_consumer", Name: "event_lag_seconds",
		Help:   "Elapsed time from source event occurrence to feature or candidate update",
		Labels: []string{"stream"},
		Buckets: []float64{
			0.1, 0.5, 1, 2, 5, 10, 30, 60, 300, 900, 3600,
		},
	})
)

// observeRecommendEventLag 按数据流（behavior/post）记录事件到特征更新的延迟。
func observeRecommendEventLag(stream string, eventTime int64, now time.Time) {
	recommendFeatureEventLag.ObserveFloat(metric.EventLagSeconds(eventTime, now), stream)
}
