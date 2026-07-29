package logic

import "github.com/zeromicro/go-zero/core/metric"

var (
	recommendPipelineTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "recommend_rpc", Name: "pipeline_total",
		Help:   "Recommendation pipeline stages by operation and outcome",
		Labels: []string{"operation", "stage", "outcome"},
	})
	recommendInferenceTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "recommend_rpc", Name: "inference_total",
		Help:   "Recommendation online inference calls by operation and outcome",
		Labels: []string{"operation", "outcome"},
	})
	recommendRecallCandidates = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "esx", Subsystem: "recommend_rpc", Name: "recall_candidates",
		Help:   "Candidate count after merging recommendation recall sources",
		Labels: []string{"operation"}, Buckets: []float64{0, 1, 5, 10, 20, 50, 100, 200, 500},
	})
	recommendResultsTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "esx", Subsystem: "recommend_rpc", Name: "results_total",
		Help:   "Recommendation responses by operation and result cardinality",
		Labels: []string{"operation", "outcome"},
	})
)

func recordPipelineStage(operation, stage string, degraded bool) {
	outcome := "success"
	if degraded {
		outcome = "degraded"
	}
	recommendPipelineTotal.Inc(operation, stage, outcome)
}

func recordRecommendationResult(operation string, count int) {
	outcome := "non_empty"
	if count == 0 {
		outcome = "empty"
	}
	recommendResultsTotal.Inc(operation, outcome)
}
