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
	embeddingIndexEventLag.ObserveFloat(embeddingEventLagSeconds(eventTime, now))
}

// embeddingEventLagSeconds 计算事件延迟秒数；缺少事件时间或时钟回拨时记为 0，避免污染直方图。
func embeddingEventLagSeconds(eventTime int64, now time.Time) float64 {
	if eventTime <= 0 {
		return 0
	}
	lag := now.Sub(time.UnixMilli(eventTime)).Seconds()
	if lag < 0 {
		return 0
	}
	return lag
}
