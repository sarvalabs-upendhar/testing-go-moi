package node

import (
	"github.com/sarvalabs/go-moi/compute"
	"github.com/sarvalabs/go-moi/consensus"
	"github.com/sarvalabs/go-moi/consensus/transport"
	"github.com/sarvalabs/go-moi/flux"
	"github.com/sarvalabs/go-moi/ixpool"
	"github.com/sarvalabs/go-moi/lattice"
	"github.com/sarvalabs/go-moi/network/p2p"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage/db"
	"github.com/sarvalabs/go-moi/syncer/agora"
	"github.com/sarvalabs/go-moi/syncer/forage"
)

type nodeMetrics struct {
	agora     *agora.Metrics
	chain     *lattice.Metrics
	flux      *flux.Metrics
	guna      *state.Metrics
	ixpool    *ixpool.Metrics
	krama     *consensus.Metrics
	syncer    *forage.Metrics
	server    *p2p.Metrics
	storage   *db.Metrics
	compute   *compute.Metrics
	transport *transport.Metrics
}

func metricProvider(nameSpace string, chainID string, metricsRequired bool) *nodeMetrics {
	if metricsRequired {
		return &nodeMetrics{
			agora:     agora.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			chain:     lattice.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			flux:      flux.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			guna:      state.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			ixpool:    ixpool.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			krama:     consensus.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			syncer:    forage.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			server:    p2p.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			storage:   db.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			compute:   compute.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			transport: transport.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
		}
	}

	return &nodeMetrics{
		agora:     agora.NilMetrics(),
		chain:     lattice.NilMetrics(),
		flux:      flux.NilMetrics(),
		guna:      state.NilMetrics(),
		ixpool:    ixpool.NilMetrics(),
		krama:     consensus.NilMetrics(),
		syncer:    forage.NilMetrics(),
		server:    p2p.NilMetrics(),
		storage:   db.NilMetrics(),
		compute:   compute.NilMetrics(),
		transport: transport.NilMetrics(),
	}
}
