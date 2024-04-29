package tree

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	TreeCacheHitCount  metrics.Counter
	TreeCacheMissCount metrics.Counter
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		TreeCacheHitCount: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "state",
			Name:      "tree_cache_hit",
			Help:      "Number of times tree cache hit",
		}, labels).With(labelsWithValues...),
		TreeCacheMissCount: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "state",
			Name:      "tree_cache_miss",
			Help:      "Number of times tree cache miss",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		TreeCacheHitCount:  discard.NewCounter(),
		TreeCacheMissCount: discard.NewCounter(),
	}
}

func (metrics *Metrics) AddTreeCacheHitCount(delta float64) {
	metrics.TreeCacheHitCount.Add(delta)
}

func (metrics *Metrics) AddTreeCacheMissCount(delta float64) {
	metrics.TreeCacheMissCount.Add(delta)
}
