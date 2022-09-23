package guna

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	ActiveStateObjects metrics.Gauge
	NumOfReverts       metrics.Counter
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		ActiveStateObjects: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "state_manager",
			Name:      "active_state_objects",
			Help:      "Number of active state objects",
		}, labels).With(labelsWithValues...),
		NumOfReverts: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "state_manager",
			Name:      "number_of_reverts",
			Help:      "Number of times state objects reverted",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		ActiveStateObjects: discard.NewGauge(),
		NumOfReverts:       discard.NewCounter(),
	}
}

// methods to capture telemetry metrics
func (metrics *Metrics) initMetrics() {
	// Initialize gauge metrics with the default value
	metrics.ActiveStateObjects.Set(0)
}

func (metrics *Metrics) captureActiveStateObjects(delta float64) {
	metrics.ActiveStateObjects.Add(delta)
}

func (metrics *Metrics) captureNumOfReverts(delta float64) {
	metrics.NumOfReverts.Add(delta)
}
