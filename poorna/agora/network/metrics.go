package network

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	ActiveConnections metrics.Gauge
	InboundDataSize   metrics.Counter
	OutboundDataSize  metrics.Counter
}

func GetPrometheusMetrics(namespace string, labels []string, labelsWithValues ...string) *Metrics {
	return &Metrics{
		ActiveConnections: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "agora",
			Name:      "active_connections",
			Help:      "Number of active sessions",
		}, labels).With(labelsWithValues...),
		InboundDataSize: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "agora",
			Name:      "inbound_data_size",
			Help:      "Amount of data received in bytes",
		}, labels).With(labelsWithValues...),
		OutboundDataSize: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "agora",
			Name:      "outbound_data_size",
			Help:      "Amount of data sent in bytes",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		ActiveConnections: discard.NewGauge(),
		InboundDataSize:   discard.NewCounter(),
		OutboundDataSize:  discard.NewCounter(),
	}
}

// methods to capture telemetry metrics
func (metrics *Metrics) initMetrics() {
	// set default value for agora network gauge metrics
	metrics.ActiveConnections.Set(0)
}

func (metrics *Metrics) captureActiveConnections(delta float64) {
	metrics.ActiveConnections.Add(delta)
}

func (metrics *Metrics) captureInboundDataSize(delta float64) {
	metrics.InboundDataSize.Add(delta)
}

func (metrics *Metrics) captureOutboundDataSize(delta float64) {
	metrics.OutboundDataSize.Add(delta)
}
