package ixpool

import (
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	PendingIxs      metrics.Gauge
	IxPoolSize      metrics.Gauge
	SlotsUsed       metrics.Gauge
	AccountWaitTime metrics.Histogram
	IxRawDup        metrics.Counter
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		PendingIxs: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "ixpool",
			Name:      "pending_interactions",
			Help:      "Pending interactions in the pool",
		}, labels).With(labelsWithValues...),
		IxPoolSize: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "ixpool",
			Name:      "interaction_pool_size",
			Help:      "Sum of all the interaction sizes in the pool",
		}, labels).With(labelsWithValues...),
		SlotsUsed: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "ixpool",
			Name:      "slots_used",
			Help:      "Number of slots consumed in the pool",
		}, labels).With(labelsWithValues...),
		AccountWaitTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "ixpool",
			Name:      "account_wait_time",
			Help:      "Time taken by an interaction associated with an account to process and complete",
			Buckets:   []float64{2, 4, 8, 10, 20, 30, 60, 120, 180, 240},
		}, labels).With(labelsWithValues...),
		IxRawDup: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "ixpool",
			Name:      "ix_raw_dup",
			Help:      "Number of ix raw duplicates",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		PendingIxs:      discard.NewGauge(),
		IxPoolSize:      discard.NewGauge(),
		SlotsUsed:       discard.NewGauge(),
		AccountWaitTime: discard.NewHistogram(),
		IxRawDup:        discard.NewCounter(),
	}
}

// methods to capture telemetry metrics
func (metrics *Metrics) initMetrics() {
	// set default value of ixpool pending interactions gauge
	metrics.PendingIxs.Set(0)
	// set default value of ixpool size gauge
	metrics.IxPoolSize.Set(0)
	// set default value of slots used
	metrics.SlotsUsed.Set(0)
}

func (metrics *Metrics) capturePendingIxs(delta float64) {
	metrics.PendingIxs.Add(delta)
}

func (metrics *Metrics) captureIxPoolSize(delta float64) {
	metrics.IxPoolSize.Add(delta)
}

func (metrics *Metrics) captureSlotsUsed(delta float64) {
	metrics.SlotsUsed.Set(delta)
}

func (metrics *Metrics) captureAccountWaitTime(requestTime time.Time, waitTime time.Time) {
	metrics.AccountWaitTime.Observe(waitTime.Sub(requestTime).Seconds())
}

func (metrics *Metrics) AddIxRawDupCount(delta float64) {
	metrics.IxRawDup.Add(delta)
}
