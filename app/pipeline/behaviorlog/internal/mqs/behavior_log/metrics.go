package behaviorlog

import (
	"time"

	metric "esx/pkg/metrics"
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

// observeBehaviorEventLag 记录行为发生到落库的延迟。
func observeBehaviorEventLag(eventTime int64, now time.Time) {
	behaviorConsumerEventLag.ObserveFloat(metric.EventLagSeconds(eventTime, now))
}
