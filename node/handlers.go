package node

import (
	"github.com/sarvalabs/go-moi/flux"
	"github.com/sarvalabs/go-moi/network/p2p"
)

type SubHandlers struct {
	core *p2p.SubHandler
	flux *flux.Randomizer
}

// setupSubHandler creates new poorna SubHandler object and setups it to node's handler's core
func (n *Node) setupSubHandler() {
	n.handlers.core = p2p.NewSubHandler(
		n.network.GetKramaID(),
		n.logger,
		n.network,
		n.senatus,
		n.network.Peers,
		n.eventMux,
		n.ixpool,
		n.chain,
		n.cfg.IxPool.EnableIxFlooding,
	)
}

// startHandlers starts syncer, core and flux(randomizer)
func (n *Node) startHandlers() error {
	n.logger.Info("Starting Sub-Handlers")

	go n.handlers.flux.Start()

	return n.handlers.core.Start(n.cfg.IxPool.EnableIxFlooding)
}

// stopHandlers stops syncer, core and flux(randomizer)
func (n *Node) stopHandlers() {
	n.logger.Info("Closing Sub-Handlers")

	if n.cfg.IxPool.EnableIxFlooding {
		n.handlers.core.Close()
	}

	n.handlers.flux.Close()
}
