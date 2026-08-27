package mqs

import (
	"time"

	"github.com/zeromicro/go-zero/core/metric"
)

var (
	watchConsumerMessages = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "assistant_watch_matcher", Name: "messages_total",
		Help: "Watch matcher messages by processing outcome", Labels: []string{"outcome"},
	})
	watchMatcherEventLag = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "assistant_watch_matcher", Name: "event_lag_seconds",
		Help:    "Elapsed time from post event occurrence to watch match",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300, 900, 3600},
	})
)

func observeWatchLag(eventTime int64, now time.Time) {
	watchMatcherEventLag.ObserveFloat(watchEventLagSeconds(eventTime, now))
}

func watchEventLagSeconds(eventTime int64, now time.Time) float64 {
	if eventTime <= 0 {
		return 0
	}
	lag := now.Sub(time.UnixMilli(eventTime)).Seconds()
	if lag < 0 {
		return 0
	}
	return lag
}
