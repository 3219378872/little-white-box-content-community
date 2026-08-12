package mqs

import (
	"time"

	"github.com/zeromicro/go-zero/core/metric"
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

func observeFeedLag(eventTime int64, now time.Time) {
	feedConsumerEventLag.ObserveFloat(feedLagSeconds(eventTime, now))
}

func feedLagSeconds(eventTime int64, now time.Time) float64 {
	if eventTime <= 0 {
		return 0
	}
	lag := now.Sub(time.UnixMilli(eventTime)).Seconds()
	if lag < 0 {
		return 0
	}
	return lag
}
