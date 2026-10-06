package metrics

import "time"

// EventLagSeconds 返回从事件发生（Unix 毫秒）到 now 的延迟秒数，供各 MQ 消费者的
// event_lag_seconds 直方图使用；缺少事件时间或时钟回拨时记为 0，避免污染直方图。
func EventLagSeconds(eventTimeMillis int64, now time.Time) float64 {
	if eventTimeMillis <= 0 {
		return 0
	}
	lag := now.Sub(time.UnixMilli(eventTimeMillis)).Seconds()
	if lag < 0 {
		return 0
	}
	return lag
}
