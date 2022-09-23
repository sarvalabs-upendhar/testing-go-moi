package node

import (
	"gitlab.com/sarvalabs/moichain/poorna/flux"
	"gitlab.com/sarvalabs/moichain/guna"
	"gitlab.com/sarvalabs/moichain/core/ixpool"
	"gitlab.com/sarvalabs/moichain/krama"
)

type nodeMetrics struct {
	flux   *flux.Metrics
	guna   *guna.Metrics
	ixpool *ixpool.Metrics
	krama  *krama.Metrics
}

func metricProvider(nameSpace string, chainID string, metricsRequired bool) *nodeMetrics {
	if metricsRequired {
		return &nodeMetrics{
			flux:   flux.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			guna:   guna.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			ixpool: ixpool.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			krama:  krama.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
		}
	}

	return &nodeMetrics{
		flux:   flux.NilMetrics(),
		guna:   guna.NilMetrics(),
		ixpool: ixpool.NilMetrics(),
		krama:  krama.NilMetrics(),
	}
}
