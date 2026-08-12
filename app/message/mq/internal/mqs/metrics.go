package mqs

import (
	"time"

	"github.com/zeromicro/go-zero/core/metric"
)

var (
	messageConsumerMessages = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "message_consumer", Name: "messages_total",
		Help: "Message notification messages by processing outcome", Labels: []string{"outcome"},
	})
	messageConsumerEventLag = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "message_consumer", Name: "event_lag_seconds",
		Help:    "Elapsed time from notification occurrence to insertion",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300, 900, 3600},
	})
)

func observeMessageLag(eventTime int64, now time.Time) {
	messageConsumerEventLag.ObserveFloat(messageLagSeconds(eventTime, now))
}

func messageLagSeconds(eventTime int64, now time.Time) float64 {
	if eventTime <= 0 {
		return 0
	}
	lag := now.Sub(time.UnixMilli(eventTime)).Seconds()
	if lag < 0 {
		return 0
	}
	return lag
}
