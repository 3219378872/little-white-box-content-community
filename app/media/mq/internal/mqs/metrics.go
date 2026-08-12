package mqs

import (
	"time"

	"github.com/zeromicro/go-zero/core/metric"
)

var (
	mediaConsumerMessages = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "media_consumer", Name: "messages_total",
		Help: "Media cleanup messages by processing outcome", Labels: []string{"outcome"},
	})
	mediaConsumerEventLag = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "media_consumer", Name: "event_lag_seconds",
		Help:    "Elapsed time from media delete occurrence to S3 object deletion",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300, 900, 3600},
	})
)

func observeMediaLag(eventTime int64, now time.Time) {
	mediaConsumerEventLag.ObserveFloat(mediaLagSeconds(eventTime, now))
}

func mediaLagSeconds(eventTime int64, now time.Time) float64 {
	if eventTime <= 0 {
		return 0
	}
	lag := now.Sub(time.UnixMilli(eventTime)).Seconds()
	if lag < 0 {
		return 0
	}
	return lag
}
