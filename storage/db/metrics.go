package db

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	DBReads  metrics.Gauge
	DBWrites metrics.Gauge
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		DBReads: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "persistence_manager",
			Name:      "db_reads",
			Help:      "Number of DB reads",
		}, labels).With(labelsWithValues...),
		DBWrites: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "persistence_manager",
			Name:      "db_writes",
			Help:      "Number of DB writes",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		DBReads:  discard.NewGauge(),
		DBWrites: discard.NewGauge(),
	}
}

func (metrics *Metrics) InitMetrics() {
	// Initialize gauge metrics of db reads with the default value
	metrics.DBReads.Set(0)
	// Initialize gauge metrics of db writes with the default value
	metrics.DBWrites.Set(0)
}

func (metrics *Metrics) CaptureDBReads(delta float64) {
	metrics.DBReads.Add(delta)
}

func (metrics *Metrics) CaptureDBWrites(delta float64) {
	metrics.DBWrites.Add(delta)
}
