package mqs

import (
	"time"

	metric "esx/pkg/metrics"
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

// observeSearchIndexLag 记录事件发生到索引更新的延迟，用于观察搜索结果的新鲜度。
func observeSearchIndexLag(eventTime int64, now time.Time) {
	searchIndexEventLag.ObserveFloat(metric.EventLagSeconds(eventTime, now))
}
