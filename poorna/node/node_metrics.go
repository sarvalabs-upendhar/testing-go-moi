package node

import (
	"github.com/sarvalabs/moichain/guna"
	"github.com/sarvalabs/moichain/ixpool"
	"github.com/sarvalabs/moichain/krama"
	"github.com/sarvalabs/moichain/lattice"
	"github.com/sarvalabs/moichain/poorna/agora"
	"github.com/sarvalabs/moichain/poorna/flux"
)

type nodeMetrics struct {
	agora  *agora.Metrics
	chain  *lattice.Metrics
	flux   *flux.Metrics
	guna   *guna.Metrics
	ixpool *ixpool.Metrics
	krama  *krama.Metrics
}

func metricProvider(nameSpace string, chainID string, metricsRequired bool) *nodeMetrics {
	if metricsRequired {
		return &nodeMetrics{
			agora:  agora.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			chain:  lattice.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			flux:   flux.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			guna:   guna.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			ixpool: ixpool.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			krama:  krama.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
		}
	}

	return &nodeMetrics{
		agora:  agora.NilMetrics(),
		chain:  lattice.NilMetrics(),
		flux:   flux.NilMetrics(),
		guna:   guna.NilMetrics(),
		ixpool: ixpool.NilMetrics(),
		krama:  krama.NilMetrics(),
	}
}
