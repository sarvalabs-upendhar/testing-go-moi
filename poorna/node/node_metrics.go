package node

import (
	"gitlab.com/sarvalabs/moichain/core/ixpool"
	"gitlab.com/sarvalabs/moichain/poorna/flux"
)

type nodeMetrics struct {
	flux   *flux.Metrics
	ixpool *ixpool.Metrics
}

// metricProvider serverMetric instance for the given ChainID and nameSpace
func metricProvider(nameSpace string, chainID string, metricsRequired bool) *nodeMetrics {
	if metricsRequired {
		return &nodeMetrics{
			flux:   flux.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			ixpool: ixpool.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
		}
	}

	return &nodeMetrics{
		flux:   flux.NilMetrics(),
		ixpool: ixpool.NilMetrics(),
	}
}
