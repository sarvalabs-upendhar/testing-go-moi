package state

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	ActiveStateObjects metrics.Gauge
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
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		ActiveStateObjects: discard.NewGauge(),
	}
}

// methods to capture telemetry metrics
func (metrics *Metrics) initMetrics() {
	// Initialize gauge metrics with the default value
	metrics.ActiveStateObjects.Set(0)
}

func (metrics *Metrics) captureActiveStateObjects(delta float64) {
	metrics.ActiveStateObjects.Set(delta)
}
