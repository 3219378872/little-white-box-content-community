package outboxx

import (
	"testing"
	"time"

	clientprometheus "github.com/prometheus/client_golang/prometheus"
	zeroprometheus "github.com/zeromicro/go-zero/core/prometheus"
)

func TestOutboxBacklogMetricsAreExported(t *testing.T) {
	zeroprometheus.Enable()
	observeBacklogMetrics("content", Backlog{Count: 7, OldestCreatedAt: 5_000}, time.UnixMilli(10_000))
	observeDeliveryLatency("content", 5_000, 10_000)
	outboxBacklogCollectionsTotal.Inc("content", "success")

	families, err := clientprometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]struct{}, len(families))
	for _, family := range families {
		found[family.GetName()] = struct{}{}
	}
	for _, name := range []string{
		"esx_outbox_backlog_count",
		"esx_outbox_oldest_age_seconds",
		"esx_outbox_backlog_collections_total",
		"esx_outbox_delivery_latency_seconds",
	} {
		if _, ok := found[name]; !ok {
			t.Errorf("metric family %q was not exported", name)
		}
	}
}

func TestObserveDeliveryLatencyIgnoresZeroAndBackdatedCreatedAt(t *testing.T) {
	observeDeliveryLatency("content", 0, 10_000)
	observeDeliveryLatency("content", 20_000, 10_000)
}
