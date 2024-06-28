package transport

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	ActiveRouters     metrics.Gauge
	ActiveMeshPeers   metrics.Gauge
	ActiveDirectPeers metrics.Gauge
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
		ActiveMeshPeers: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "transport",
			Name:      "active_mesh_peers",
			Help:      "Number of active mesh peers",
		}, labels).With(labelsWithValues...),
		ActiveDirectPeers: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "transport",
			Name:      "active_direct_peers",
			Help:      "Number of active direct peers",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		ActiveRouters:     discard.NewGauge(),
		ActiveMeshPeers:   discard.NewGauge(),
		ActiveDirectPeers: discard.NewGauge(),
	}
}

func (metrics *Metrics) captureActiveRouters(delta float64) {
	metrics.ActiveRouters.Add(delta)
}

func (metrics *Metrics) captureActiveMeshPeers(delta float64) {
	metrics.ActiveMeshPeers.Add(delta)
}

func (metrics *Metrics) captureActiveDirectPeers(delta float64) {
	metrics.ActiveDirectPeers.Add(delta)
}
