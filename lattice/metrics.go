package lattice

import (
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	StatefulTesseractAdditionTime metrics.Histogram
	StatefulTesseractCounter      metrics.Counter
	IxnsPerTesseract              metrics.Histogram
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		StatefulTesseractAdditionTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "chain_manager",
			Name:      "stateful_tesseract_addition_time",
			Help:      "Time taken to add the created tesseract",
			Buckets:   []float64{5, 10, 15, 20, 25, 30, 50, 100},
		}, labels).With(labelsWithValues...),
		StatefulTesseractCounter: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "chain_manager",
			Name:      "stateful_tesseract_counter",
			Help:      "Number of tesseracts created",
		}, labels).With(labelsWithValues...),
		IxnsPerTesseract: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "chain_manager",
			Name:      "ixns_per_tesseract",
			Help:      "Number ixns in a tesseract",
			Buckets:   []float64{5, 10, 20, 40, 60, 80, 100, 150, 200, 300, 400},
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		StatefulTesseractAdditionTime: discard.NewHistogram(),
		StatefulTesseractCounter:      discard.NewCounter(),
		IxnsPerTesseract:              discard.NewHistogram(),
	}
}

func (metrics *Metrics) captureStatefulTesseractAdditionTime(tsAdditionInitTime time.Time) {
	metrics.StatefulTesseractAdditionTime.Observe(time.Since(tsAdditionInitTime).Seconds())
}

func (metrics *Metrics) captureStatefulTesseractCounter(delta float64) {
	metrics.StatefulTesseractCounter.Add(delta)
}

func (metrics *Metrics) captureIxnsPerTesseract(count float64) {
	metrics.IxnsPerTesseract.Observe(count)
}
