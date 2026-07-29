package behaviorlog

import (
	"time"

	"github.com/zeromicro/go-zero/core/metric"
)

var (
	behaviorConsumerMessages = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "behavior_log_consumer", Name: "messages_total",
		Help: "Behavior log messages by terminal processing outcome", Labels: []string{"outcome"},
	})
	behaviorConsumerEventLag = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "behavior_log_consumer", Name: "event_lag_seconds",
		Help:    "Elapsed time from behavior occurrence to durable ClickHouse processing",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300, 900, 3600},
	})
)

func observeBehaviorEventLag(eventTime int64, now time.Time) {
	behaviorConsumerEventLag.ObserveFloat(eventLagSeconds(eventTime, now))
}

func eventLagSeconds(eventTime int64, now time.Time) float64 {
	if eventTime <= 0 {
		return 0
	}
	lag := now.Sub(time.UnixMilli(eventTime)).Seconds()
	if lag < 0 {
		return 0
	}
	return lag
}
