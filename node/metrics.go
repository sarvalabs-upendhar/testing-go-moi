package node

import (
	"github.com/sarvalabs/moichain/consensus"
	"github.com/sarvalabs/moichain/flux"
	"github.com/sarvalabs/moichain/ixpool"
	"github.com/sarvalabs/moichain/lattice"
	"github.com/sarvalabs/moichain/network/p2p"
	"github.com/sarvalabs/moichain/state"
	"github.com/sarvalabs/moichain/syncer/agora"
	"github.com/sarvalabs/moichain/syncer/forage"
)

type nodeMetrics struct {
	agora  *agora.Metrics
	chain  *lattice.Metrics
	flux   *flux.Metrics
	guna   *state.Metrics
	ixpool *ixpool.Metrics
	krama  *consensus.Metrics
	syncer *forage.Metrics
	server *p2p.Metrics
}

func metricProvider(nameSpace string, chainID string, metricsRequired bool) *nodeMetrics {
	if metricsRequired {
		return &nodeMetrics{
			agora:  agora.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			chain:  lattice.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			flux:   flux.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			guna:   state.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			ixpool: ixpool.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			krama:  consensus.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			syncer: forage.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			server: p2p.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
		}
	}

	return &nodeMetrics{
		agora:  agora.NilMetrics(),
		chain:  lattice.NilMetrics(),
		flux:   flux.NilMetrics(),
		guna:   state.NilMetrics(),
		ixpool: ixpool.NilMetrics(),
		krama:  consensus.NilMetrics(),
		syncer: forage.NilMetrics(),
		server: p2p.NilMetrics(),
	}
}
