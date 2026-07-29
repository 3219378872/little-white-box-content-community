package mqs

import (
	"time"

	"github.com/zeromicro/go-zero/core/metric"
)

var (
	searchConsumerMessages = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "search_index_consumer", Name: "messages_total",
		Help: "Search index messages by processing outcome", Labels: []string{"outcome"},
	})
	searchIndexEventLag = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "search_index_consumer", Name: "event_lag_seconds",
		Help:    "Elapsed time from post event occurrence to Elasticsearch index update",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300, 900, 3600},
	})
)

func observeSearchIndexLag(eventTime int64, now time.Time) {
	searchIndexEventLag.ObserveFloat(searchEventLagSeconds(eventTime, now))
}

func searchEventLagSeconds(eventTime int64, now time.Time) float64 {
	if eventTime <= 0 {
		return 0
	}
	lag := now.Sub(time.UnixMilli(eventTime)).Seconds()
	if lag < 0 {
		return 0
	}
	return lag
}
