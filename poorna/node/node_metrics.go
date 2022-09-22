package node

import (
	"gitlab.com/sarvalabs/moichain/krama/ics"
	"gitlab.com/sarvalabs/moichain/poorna/flux"
)

type nodeMetrics struct {
	ics  *ics.ICSMetrics
	flux *flux.Metrics
}

// metricProvider serverMetric instance for the given ChainID and nameSpace
func metricProvider(nameSpace string, chainID string, metricsRequired bool) *nodeMetrics {
	if metricsRequired {
		return &nodeMetrics{
			ics:  ics.GetICSPrometheusMetrics(nameSpace, "chain_id", chainID),
			flux: flux.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
		}
	}

	return &nodeMetrics{
		ics:  ics.ICSNilMetrics(),
		flux: flux.NilMetrics(),
	}
}
