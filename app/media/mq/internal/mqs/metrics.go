package mqs

import (
	"time"

	"esx/pkg/event"
	metric "esx/pkg/metrics"
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

// mediaEventMillis 返回事件发生时间（毫秒）。EventTime 已是毫秒；旧消息只有秒级的 DeletedAt，
// 换算成毫秒后再参与延迟计算，避免把秒当毫秒得到接近 0 的时间戳。
func mediaEventMillis(e event.MediaDeletedEvent) int64 {
	if e.EventTime > 0 {
		return e.EventTime
	}
	if e.DeletedAt > 0 {
		return e.DeletedAt * 1000
	}
	return 0
}

// observeMediaLag 记录媒体删除到对象清理完成的延迟。
func observeMediaLag(e event.MediaDeletedEvent, now time.Time) {
	mediaConsumerEventLag.ObserveFloat(mediaLagSeconds(e, now))
}

// mediaLagSeconds 计算媒体删除事件的延迟秒数，事件时间先经 mediaEventMillis 统一为毫秒。
func mediaLagSeconds(e event.MediaDeletedEvent, now time.Time) float64 {
	return metric.EventLagSeconds(mediaEventMillis(e), now)
}
