package chain

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"time"
)

type Metrics struct {
	SignatureVerificationTime      metrics.Histogram
	StatefulTesseractAdditionTime  metrics.Histogram
	StatelessTesseractAdditionTime metrics.Histogram
	StatefulTesseractCounter       metrics.Counter
	StatelessTesseractCounter      metrics.Counter
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
		}, labels).With(labelsWithValues...),
		StatefulTesseractAdditionTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "chain_manager",
			Name:      "stateful_tesseract_addition_time",
			Help:      "Time taken to add the created tesseract",
		}, labels).With(labelsWithValues...),
		StatelessTesseractAdditionTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "chain_manager",
			Name:      "stateless_tesseract_addition_time",
			Help:      "Time taken to add the received tesseract",
		}, labels).With(labelsWithValues...),
		StatefulTesseractCounter: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "chain_manager",
			Name:      "stateful_tesseract_counter",
			Help:      "Number of tesseracts created",
		}, labels).With(labelsWithValues...),
		StatelessTesseractCounter: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "chain_manager",
			Name:      "stateless_tesseract_counter",
			Help:      "Number of tesseracts received",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		SignatureVerificationTime:      discard.NewHistogram(),
		StatefulTesseractAdditionTime:  discard.NewHistogram(),
		StatelessTesseractAdditionTime: discard.NewHistogram(),
		StatefulTesseractCounter:       discard.NewCounter(),
		StatelessTesseractCounter:      discard.NewCounter(),
	}
}

// methods to capture telemetry metrics
func (metrics *Metrics) captureSignatureVerificationTime(verificationInitTime time.Time) {
	metrics.SignatureVerificationTime.Observe(time.Since(verificationInitTime).Seconds())
}

func (metrics *Metrics) captureStatefulTesseractAdditionTime(tsAdditionInitTime time.Time) {
	metrics.StatefulTesseractAdditionTime.Observe(time.Since(tsAdditionInitTime).Seconds())
}

func (metrics *Metrics) captureStatelessTesseractAdditionTime(tsAdditionInitTime time.Time) {
	metrics.StatelessTesseractAdditionTime.Observe(time.Since(tsAdditionInitTime).Seconds())
}

func (metrics *Metrics) captureStatefulTesseractCounter(delta float64) {
	metrics.StatefulTesseractCounter.Add(delta)
}

func (metrics *Metrics) captureStatelessTesseractCounter(delta float64) {
	metrics.StatelessTesseractCounter.Add(delta)
}
