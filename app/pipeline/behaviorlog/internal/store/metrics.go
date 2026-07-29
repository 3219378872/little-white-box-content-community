package store

import (
	"time"

	"github.com/zeromicro/go-zero/core/metric"
)

var clickHouseWriteSeconds = metric.NewHistogramVec(&metric.HistogramVecOpts{
	Namespace: "esx", Subsystem: "behavior_log_store", Name: "write_seconds",
	Help:   "ClickHouse behavior store write latency by operation and outcome",
	Labels: []string{"operation", "outcome"},
	Buckets: []float64{
		0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5,
	},
})

func observeClickHouseWrite(startedAt time.Time, operation string, err error) {
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	clickHouseWriteSeconds.ObserveFloat(time.Since(startedAt).Seconds(), operation, outcome)
}
