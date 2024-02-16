package compute

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	NumOfExecutionFailure metrics.Counter
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		NumOfExecutionFailure: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "compute",
			Name:      "number_of_execution_failure",
			Help:      "Number of times interaction execution failed",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		NumOfExecutionFailure: discard.NewCounter(),
	}
}

// methods to capture telemetry metrics
func (metrics *Metrics) captureNumOfExecutionFailure(delta float64) {
	metrics.NumOfExecutionFailure.Add(delta)
}
