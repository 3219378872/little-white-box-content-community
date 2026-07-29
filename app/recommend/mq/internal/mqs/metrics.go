package mqs

import (
	"time"

	"github.com/zeromicro/go-zero/core/metric"
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

func observeRecommendEventLag(stream string, eventTime int64, now time.Time) {
	recommendFeatureEventLag.ObserveFloat(nonNegativeEventLagSeconds(eventTime, now), stream)
}

func nonNegativeEventLagSeconds(eventTime int64, now time.Time) float64 {
	if eventTime <= 0 {
		return 0
	}
	lag := now.Sub(time.UnixMilli(eventTime)).Seconds()
	if lag < 0 {
		return 0
	}
	return lag
}
