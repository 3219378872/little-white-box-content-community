// Package metrics owns the application's Prometheus metric registration.
package metrics

import "github.com/prometheus/client_golang/prometheus"

// CounterVecOpts names a labelled counter.
type CounterVecOpts struct {
	Namespace, Subsystem, Name, Help string
	Labels                           []string
}

// HistogramVecOpts names a labelled histogram and its buckets.
type HistogramVecOpts struct {
	Namespace, Subsystem, Name, Help string
	Labels                           []string
	Buckets                          []float64
}

// GaugeVecOpts names a labelled gauge; the wrapper types below take label values positionally.
type GaugeVecOpts = CounterVecOpts
type CounterVec struct{ v *prometheus.CounterVec }
type GaugeVec struct{ v *prometheus.GaugeVec }
type HistogramVec struct{ v *prometheus.HistogramVec }

// NewCounterVec creates and registers a counter; duplicate registration panics at startup.
func NewCounterVec(o *CounterVecOpts) *CounterVec {
	v := prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: o.Namespace, Subsystem: o.Subsystem, Name: o.Name, Help: o.Help}, o.Labels)
	prometheus.MustRegister(v)
	return &CounterVec{v}
}

// NewGaugeVec creates and registers a gauge.
func NewGaugeVec(o *GaugeVecOpts) *GaugeVec {
	v := prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: o.Namespace, Subsystem: o.Subsystem, Name: o.Name, Help: o.Help}, o.Labels)
	prometheus.MustRegister(v)
	return &GaugeVec{v}
}

// NewHistogramVec creates and registers a histogram.
func NewHistogramVec(o *HistogramVecOpts) *HistogramVec {
	v := prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: o.Namespace, Subsystem: o.Subsystem, Name: o.Name, Help: o.Help, Buckets: o.Buckets}, o.Labels)
	prometheus.MustRegister(v)
	return &HistogramVec{v}
}

// Inc, Add, Set and Observe record a sample for the given label values.
func (c *CounterVec) Inc(labels ...string)                { c.v.WithLabelValues(labels...).Inc() }
func (c *CounterVec) Add(v float64, labels ...string)     { c.v.WithLabelValues(labels...).Add(v) }
func (c *GaugeVec) Set(v float64, labels ...string)       { c.v.WithLabelValues(labels...).Set(v) }
func (c *HistogramVec) Observe(v int64, labels ...string) { c.ObserveFloat(float64(v), labels...) }

// ObserveFloat records a fractional histogram sample.
func (c *HistogramVec) ObserveFloat(v float64, labels ...string) {
	c.v.WithLabelValues(labels...).Observe(v)
}
