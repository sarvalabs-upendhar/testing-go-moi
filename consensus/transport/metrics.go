package transport

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	ActiveRouters metrics.Gauge
	ActivePeers   metrics.Gauge
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		ActiveRouters: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "transport",
			Name:      "active_routers",
			Help:      "Number of active context routers",
		}, labels).With(labelsWithValues...),
		ActivePeers: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "transport",
			Name:      "active_peers",
			Help:      "Number of active peers in peer set",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		ActiveRouters: discard.NewGauge(),
		ActivePeers:   discard.NewGauge(),
	}
}

func (metrics *Metrics) captureActiveRouters(delta float64) {
	metrics.ActiveRouters.Add(delta)
}

func (metrics *Metrics) captureActivePeers(delta float64) {
	metrics.ActivePeers.Add(delta)
}
