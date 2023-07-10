package ixpool

import (
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	prometheus "github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	PendingIxs      metrics.Gauge
	IxPoolSize      metrics.Gauge
	SlotsUsed       metrics.Gauge
	AccountWaitTime metrics.Histogram
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
			Name:      "pending_transactions",
			Help:      "Pending transactions in the pool",
		}, labels).With(labelsWithValues...),
		IxPoolSize: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "ixpool",
			Name:      "interaction_pool_size",
			Help:      "Sum of all the transaction sizes in the pool",
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
			Help:      "Time taken by a transaction associated with an account to process and complete",
			Buckets:   []float64{500, 1000, 1500, 2000, 2500, 3000, 3500, 4000, 4500, 5000},
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		PendingIxs:      discard.NewGauge(),
		IxPoolSize:      discard.NewGauge(),
		SlotsUsed:       discard.NewGauge(),
		AccountWaitTime: discard.NewHistogram(),
	}
}

// methods to capture telemetry metrics
func (metrics *Metrics) initMetrics() {
	// set default value of ixpool pending transactions gauge
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
