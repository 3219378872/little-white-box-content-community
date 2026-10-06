package mqs

import (
	"time"

	metric "esx/pkg/metrics"
)

var (
	embeddingConsumerMessages = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "embedding_index_consumer", Name: "messages_total",
		Help: "Embedding index messages by processing outcome", Labels: []string{"outcome"},
	})
	embeddingIndexEventLag = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "embedding_index_consumer", Name: "event_lag_seconds",
		Help:    "Elapsed time from post event occurrence to Milvus vector update",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300, 900, 3600},
	})
)

// observeEmbeddingIndexLag 记录事件发生到向量更新的延迟。
func observeEmbeddingIndexLag(eventTime int64, now time.Time) {
	embeddingIndexEventLag.ObserveFloat(metric.EventLagSeconds(eventTime, now))
}
