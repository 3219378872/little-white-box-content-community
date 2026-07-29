package logic

import (
	"testing"

	clientprometheus "github.com/prometheus/client_golang/prometheus"
	zeroprometheus "github.com/zeromicro/go-zero/core/prometheus"
)

func TestBehaviorMetricsExportMQPublishOutcomes(t *testing.T) {
	zeroprometheus.Enable()
	behaviorRecordTotal.Inc("accepted")
	behaviorMQPublishTotal.Inc("success")
	behaviorMQPublishTotal.Inc("failure")

	families, err := clientprometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]struct{}, len(families))
	for _, family := range families {
		found[family.GetName()] = struct{}{}
	}
	for _, name := range []string{
		"esx_behavior_rpc_record_events_total",
		"esx_behavior_rpc_mq_publish_total",
	} {
		if _, ok := found[name]; !ok {
			t.Errorf("metric family %q was not exported", name)
		}
	}
}
