package node

import (
	"gitlab.com/sarvalabs/moichain/core/chain"
	"gitlab.com/sarvalabs/moichain/core/ixpool"
	"gitlab.com/sarvalabs/moichain/guna"
	"gitlab.com/sarvalabs/moichain/krama"
	"gitlab.com/sarvalabs/moichain/poorna/flux"
)

type nodeMetrics struct {
	chain  *chain.Metrics
	flux   *flux.Metrics
	guna   *guna.Metrics
	ixpool *ixpool.Metrics
	krama  *krama.Metrics
}

func metricProvider(nameSpace string, chainID string, metricsRequired bool) *nodeMetrics {
	if metricsRequired {
		return &nodeMetrics{
			chain:  chain.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			flux:   flux.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			guna:   guna.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			ixpool: ixpool.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			krama:  krama.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
		}
	}

	return &nodeMetrics{
		chain:  chain.NilMetrics(),
		flux:   flux.NilMetrics(),
		guna:   guna.NilMetrics(),
		ixpool: ixpool.NilMetrics(),
		krama:  krama.NilMetrics(),
	}
}
