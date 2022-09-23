package flux

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	PendingSlots  metrics.Gauge
	NumOfRequests metrics.Counter
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		PendingSlots: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "flex",
			Name:      "pending_slots",
			Help:      "Number of pending slots in the randomizer",
		}, labels).With(labelsWithValues...),
		NumOfRequests: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "flex",
			Name:      "number_of_requests",
			Help:      "Number of requests received",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		PendingSlots:  discard.NewGauge(),
		NumOfRequests: discard.NewCounter(),
	}
}
