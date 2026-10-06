package outboxx

import (
	"time"

	metric "esx/pkg/metrics"
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
	outboxPurgedTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "outbox", Name: "purged_total",
		Help:   "Sent transactional outbox events deleted after the retention window",
		Labels: []string{"service"},
	})
	outboxDeliveryLatencySeconds = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "outbox", Name: "delivery_latency_seconds",
		Help:    "Elapsed time from outbox event creation to broker publish acknowledgment",
		Labels:  []string{"service"},
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 900, 3600},
	})
)

// observeBacklogMetrics publishes the pending count and the age of the oldest event.
func observeBacklogMetrics(service string, backlog Backlog, now time.Time) {
	outboxBacklogCount.Set(float64(backlog.Count), service)
	outboxOldestAgeSeconds.Set(backlogAgeSeconds(backlog, now), service)
}

// observeDeliveryLatency records commit-to-publish latency; clock skew samples are dropped.
func observeDeliveryLatency(service string, createdAtMillis, nowMillis int64) {
	if createdAtMillis <= 0 || nowMillis < createdAtMillis {
		return
	}
	outboxDeliveryLatencySeconds.ObserveFloat(float64(nowMillis-createdAtMillis)/1000.0, service)
}

// backlogAgeSeconds is the age of the oldest undelivered event, 0 when the backlog is empty.
func backlogAgeSeconds(backlog Backlog, now time.Time) float64 {
	if backlog.Count <= 0 {
		return 0
	}
	return metric.EventLagSeconds(backlog.OldestCreatedAt, now)
}
