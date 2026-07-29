package outboxx

import (
	"time"

	"github.com/zeromicro/go-zero/core/metric"
)

var (
	outboxBacklogCount = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: "esx", Subsystem: "outbox", Name: "backlog_count",
		Help:   "Number of pending, processing, or retrying transactional outbox events",
		Labels: []string{"service"},
	})
	outboxOldestAgeSeconds = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: "esx", Subsystem: "outbox", Name: "oldest_age_seconds",
		Help:   "Age in seconds of the oldest transactional outbox event",
		Labels: []string{"service"},
	})
	outboxBacklogCollectionsTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "outbox", Name: "backlog_collections_total",
		Help:   "Transactional outbox backlog collection attempts by outcome",
		Labels: []string{"service", "outcome"},
	})
)

func observeBacklogMetrics(service string, backlog Backlog, now time.Time) {
	outboxBacklogCount.Set(float64(backlog.Count), service)
	outboxOldestAgeSeconds.Set(backlogAgeSeconds(backlog, now), service)
}

func backlogAgeSeconds(backlog Backlog, now time.Time) float64 {
	if backlog.Count <= 0 || backlog.OldestCreatedAt <= 0 {
		return 0
	}
	age := now.Sub(time.UnixMilli(backlog.OldestCreatedAt)).Seconds()
	if age < 0 {
		return 0
	}
	return age
}
