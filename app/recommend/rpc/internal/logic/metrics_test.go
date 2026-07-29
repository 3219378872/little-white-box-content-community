package logic

import (
	"testing"

	clientprometheus "github.com/prometheus/client_golang/prometheus"
	zeroprometheus "github.com/zeromicro/go-zero/core/prometheus"
)

func TestRecommendMetricsAreExported(t *testing.T) {
	zeroprometheus.Enable()
	recordPipelineStage("posts", "recall", true)
	recommendInferenceTotal.Inc("posts", "timeout")
	recommendRecallCandidates.Observe(12, "posts")
	recordRecommendationResult("posts", 0)

	assertRecommendMetricFamilies(t, []string{
		"esx_recommend_rpc_pipeline_total",
		"esx_recommend_rpc_inference_total",
		"esx_recommend_rpc_recall_candidates",
		"esx_recommend_rpc_results_total",
	})
}

func assertRecommendMetricFamilies(t *testing.T, expected []string) {
	t.Helper()
	families, err := clientprometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]struct{}, len(families))
	for _, family := range families {
		found[family.GetName()] = struct{}{}
	}
	for _, name := range expected {
		if _, ok := found[name]; !ok {
			t.Errorf("metric family %q was not exported", name)
		}
	}
}
