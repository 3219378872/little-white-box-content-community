// Package metrics owns the application's Prometheus metric registration.
package metrics

import "github.com/prometheus/client_golang/prometheus"

type CounterVecOpts struct {
	Namespace, Subsystem, Name, Help string
	Labels                           []string
}
type HistogramVecOpts struct {
	Namespace, Subsystem, Name, Help string
	Labels                           []string
	Buckets                          []float64
}
type GaugeVecOpts = CounterVecOpts
type CounterVec struct{ v *prometheus.CounterVec }
type GaugeVec struct{ v *prometheus.GaugeVec }
type HistogramVec struct{ v *prometheus.HistogramVec }

func NewCounterVec(o *CounterVecOpts) *CounterVec {
	v := prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: o.Namespace, Subsystem: o.Subsystem, Name: o.Name, Help: o.Help}, o.Labels)
	prometheus.MustRegister(v)
	return &CounterVec{v}
}
func NewGaugeVec(o *GaugeVecOpts) *GaugeVec {
	v := prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: o.Namespace, Subsystem: o.Subsystem, Name: o.Name, Help: o.Help}, o.Labels)
	prometheus.MustRegister(v)
	return &GaugeVec{v}
}
func NewHistogramVec(o *HistogramVecOpts) *HistogramVec {
	v := prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: o.Namespace, Subsystem: o.Subsystem, Name: o.Name, Help: o.Help, Buckets: o.Buckets}, o.Labels)
	prometheus.MustRegister(v)
	return &HistogramVec{v}
}
func (c *CounterVec) Inc(labels ...string)                { c.v.WithLabelValues(labels...).Inc() }
func (c *CounterVec) Add(v float64, labels ...string)     { c.v.WithLabelValues(labels...).Add(v) }
func (c *GaugeVec) Set(v float64, labels ...string)       { c.v.WithLabelValues(labels...).Set(v) }
func (c *HistogramVec) Observe(v int64, labels ...string) { c.ObserveFloat(float64(v), labels...) }
func (c *HistogramVec) ObserveFloat(v float64, labels ...string) {
	c.v.WithLabelValues(labels...).Observe(v)
}
