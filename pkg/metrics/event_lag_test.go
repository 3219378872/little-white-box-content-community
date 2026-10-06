package metrics

import (
	"testing"
	"time"
)

func TestEventLagSeconds(t *testing.T) {
	now := time.Unix(10, 0)
	cases := []struct {
		name      string
		eventTime int64
		want      float64
	}{
		{name: "whole seconds", eventTime: now.Add(-time.Second).UnixMilli(), want: 1},
		{name: "millisecond precision", eventTime: now.Add(-2500 * time.Millisecond).UnixMilli(), want: 2.5},
		{name: "future event clamps to zero", eventTime: now.Add(time.Second).UnixMilli(), want: 0},
		{name: "missing event time", eventTime: 0, want: 0},
		{name: "negative event time", eventTime: -1, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EventLagSeconds(tc.eventTime, now); got != tc.want {
				t.Fatalf("EventLagSeconds()=%v want=%v", got, tc.want)
			}
		})
	}
}
