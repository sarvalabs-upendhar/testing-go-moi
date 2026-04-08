package state

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/sarvalabs/go-moi/state/tree"
)

type Metrics struct {
	ActiveStateObjects   metrics.Gauge
	ObjectCacheHitCount  metrics.Counter
	ObjectCacheMissCount metrics.Counter
	TreeMetrics          *tree.Metrics
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		ActiveStateObjects: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "state",
			Name:      "active_state_objects",
			Help:      "Number of active state objects",
		}, labels).With(labelsWithValues...),
		ObjectCacheHitCount: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "state",
			Name:      "object_cache_hit",
			Help:      "Number of times object cache hit",
		}, labels).With(labelsWithValues...),
		ObjectCacheMissCount: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "state",
			Name:      "object_cache_miss",
			Help:      "Number of times object cache miss",
		}, labels).With(labelsWithValues...),
		TreeMetrics: tree.GetPrometheusMetrics(namespace, labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		ActiveStateObjects:   discard.NewGauge(),
		ObjectCacheHitCount:  discard.NewCounter(),
		ObjectCacheMissCount: discard.NewCounter(),
		TreeMetrics:          tree.NilMetrics(),
	}
}

func (metrics *Metrics) InitMetrics() {
	// Initialize gauge metrics with the default value
	metrics.ActiveStateObjects.Set(0)
}

func (metrics *Metrics) CaptureActiveStateObjects(delta float64) {
	metrics.ActiveStateObjects.Set(delta)
}

func (metrics *Metrics) AddObjectCacheHitCount(delta float64) {
	metrics.ObjectCacheHitCount.Add(delta)
}

func (metrics *Metrics) AddObjectCacheMissCount(delta float64) {
	metrics.ObjectCacheMissCount.Add(delta)
}
