package lattice

import (
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	SignatureVerificationTime     metrics.Histogram
	StatefulTesseractAdditionTime metrics.Histogram
	StatefulTesseractCounter      metrics.Counter
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		SignatureVerificationTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "chain_manager",
			Name:      "signature_verification_time",
			Help:      "Time taken to verify tesseract signature",
			Buckets:   []float64{5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 100},
		}, labels).With(labelsWithValues...),
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
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		SignatureVerificationTime:     discard.NewHistogram(),
		StatefulTesseractAdditionTime: discard.NewHistogram(),
		StatefulTesseractCounter:      discard.NewCounter(),
	}
}

// methods to capture telemetry metrics
func (metrics *Metrics) captureSignatureVerificationTime(verificationInitTime time.Time) {
	metrics.SignatureVerificationTime.Observe(time.Since(verificationInitTime).Seconds())
}

func (metrics *Metrics) captureStatefulTesseractAdditionTime(tsAdditionInitTime time.Time) {
	metrics.StatefulTesseractAdditionTime.Observe(time.Since(tsAdditionInitTime).Seconds())
}

func (metrics *Metrics) captureStatefulTesseractCounter(delta float64) {
	metrics.StatefulTesseractCounter.Add(delta)
}
