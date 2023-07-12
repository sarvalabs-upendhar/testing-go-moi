package node

import (
	"github.com/sarvalabs/go-moi/flux"
	"github.com/sarvalabs/go-moi/network/p2p"
	"github.com/sarvalabs/go-moi/syncer/forage"
)

type SubHandlers struct {
	syncer *forage.Syncer
	core   *p2p.SubHandler
	flux   *flux.Randomizer
}

// setupSubHandler creates new poorna SubHandler object and setups it to node's handler's core
func (n *Node) setupSubHandler() {
	n.handlers.core = p2p.NewSubHandler(
		n.ctx,
		n.network.GetKramaID(),
		n.logger,
		n.network,
		n.network.Peers,
		n.eventMux,
		n.ixpool,
		n.chain,
	)
}

// startHandlers starts syncer, core and flux(randomizer)
func (n *Node) startHandlers() {
	n.logger.Info("Starting sub-handlers")

	go n.handlers.core.Start()
	go n.handlers.syncer.Start()
	go n.handlers.flux.Start()
}

// stopHandlers stops syncer, core and flux(randomizer)
func (n *Node) stopHandlers() {
	n.handlers.core.Close()
	n.handlers.syncer.Close()
	n.handlers.flux.Close()
}
