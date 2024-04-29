package p2p

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"

	"github.com/libp2p/go-libp2p/core/peer"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/sarvalabs/go-moi/common/config"
)

const (
	// advertiseInterval is the time interval between consecutive advertisements
	advertiseInterval = 3 * time.Hour

	// discovery timeout is the timeout for one batch of discovery
	discoveryTimeout = 1 * time.Minute

	// searchTimeout timeout is the timeout for locating a peer in the Kademlia DHT.
	searchTimeout = 1 * time.Minute

	connectionTimeout = 800 * time.Millisecond
)

// DiscoveryService is a struct that manages peer discovery.
type DiscoveryService struct {
	server    *Server
	discovery *discovery.RoutingDiscovery
	peerChan  chan peer.AddrInfo
}

// NewDiscoveryService creates a new instance of DiscoveryService.
func NewDiscoveryService(server *Server) *DiscoveryService {
	return &DiscoveryService{
		server:    server,
		discovery: discovery.NewRoutingDiscovery(server.kadDHT),
		peerChan:  make(chan peer.AddrInfo),
	}
}

// advertise periodically announces the presence of the node to the network.
func (ds *DiscoveryService) advertise() {
	for {
		ds.server.logger.Info("Announcing ourselves")

		_, err := ds.discovery.Advertise(ds.server.ctx, string(config.MOIProtocolStream))
		if err != nil {
			ds.server.logger.Error("Failed to advertise the rendezvous string to the discovery service", "err", err)

			time.Sleep(5 * time.Second) // we should wait until network boots up

			continue
		}

		select {
		case <-time.After(advertiseInterval):
		case <-ds.server.ctx.Done():
			return
		}
	}
}

// discover periodically discovers other peers that are advertising themselves
func (ds *DiscoveryService) discover() {
	ds.server.logger.Info("Starting discovery routine")

	for {
		select {
		case <-time.After(ds.server.cfg.DiscoveryInterval):
		case <-ds.server.ctx.Done():
			return
		}

		if err := ds.findPeers(ds.server.ctx); err != nil {
			ds.server.logger.Error("Handle discovery", "err", err)
		}
	}
}

// findPeer searches for a specific peer using its ID and returns the peer's address information if found.
func (ds *DiscoveryService) findPeer(ctx context.Context, peerID peer.ID) (peer.AddrInfo, error) {
	searchCtx, searchCancel := context.WithTimeout(ctx, searchTimeout)
	defer searchCancel()

	return ds.server.kadDHT.FindPeer(searchCtx, peerID)
}

// findPeers retrieves information about other peers from the network.
func (ds *DiscoveryService) findPeers(ctx context.Context) error {
	discCtx, discCancel := context.WithTimeout(ctx, discoveryTimeout)
	defer discCancel()

	// Retrieve a channel of peer addresses from the discovery service
	peerChan, err := ds.discovery.FindPeers(discCtx, string(config.MOIProtocolStream))
	if err != nil {
		return err
	}

	for peerInfo := range peerChan {
		ds.peerChan <- peerInfo
	}

	return nil
}

// handleDiscoveredPeers processes information about discovered peers, ensuring that valid peers are added to
// the network, while avoiding self-connections, connection limitations, and potential errors during the
// connection setup process.
func (ds *DiscoveryService) handleDiscoveredPeers() {
	ds.server.logger.Info("Handling the discovered peers")

	for {
		select {
		case peerInfo := <-ds.peerChan:
			// Skip the iteration if the peer address doesn't exist
			if len(peerInfo.Addrs) == 0 {
				continue
			}

			// Skip iteration if the peer addresses points to self
			if peerInfo.ID == ds.server.host.ID() {
				continue
			}

			// Skip iteration if the peer already exists in the peer set
			if ds.server.Peers.ContainsPeer(peerInfo.ID) {
				continue
			}

			// Skip the iteration if the peer is under cool down period
			if ds.server.ConnManager.coolDownCache.Has(peerInfo.ID) {
				continue
			}

			kramaID, rtt, err := ds.server.ConnManager.retrieveRTTAndRefreshSenatus(peerInfo)
			if err != nil {
				ds.server.logger.Error("Failed to retrieve rtt and refresh senatus", "err", err)

				continue
			}

			if !ds.server.cfg.NoDiscovery {
				err = ds.server.ConnManager.ConnectAndRegisterPeer(ds.server.ctx, peerInfo, kramaID, rtt)
				if err != nil {
					/*
						Skip iteration on,
						* Outbound connection limit failure
						* Stream setup failure
						* Error fetching ntq
						* Handshake failure
					*/
					ds.server.ConnManager.coolDownCache.Add(peerInfo.ID)

					if !errors.Is(err, common.ErrOutboundConnLimit) {
						ds.server.logger.Trace("Failed to connect peer", "peerid", peerInfo.ID, "err", err)
					}

					continue
				}
			}

		case <-ds.server.ctx.Done():
			return
		}
	}
}

// handlePeerDiscoveryRequest listens for peer discovery events and handles them.
func (ds *DiscoveryService) handlePeerDiscoveryRequest() {
	sub := ds.server.mux.Subscribe(DiscoverPeerEvent{})
	defer sub.Unsubscribe()

	ds.server.logger.Info("Handling the discover peer event")

	for event := range sub.Chan() {
		event, ok := event.Data.(DiscoverPeerEvent)
		if ok {
			// Retrieve peer information from the DHT based on the event ID
			peerInfo, err := ds.findPeer(ds.server.ctx, event.ID)
			if err != nil {
				ds.server.logger.Error("Failed to find peer in dht", event.ID)

				continue
			}

			kramaID, rtt, err := ds.server.ConnManager.pingPeer(peerInfo)
			if err != nil {
				ds.server.logger.Error("Failed to ping peer", "err", err, "peer-id", event.ID)

				continue
			}

			err = ds.server.ConnManager.refreshSenatus(peerInfo, kramaID, rtt)
			if err != nil {
				ds.server.logger.Error("Failed to refresh senatus", "err", err, "peer-id", event.ID)

				continue
			}
		}
	}
}

// Start initiates the DiscoveryService, allowing the node to discover and advertise to peers.
func (ds *DiscoveryService) Start() {
	go ds.advertise()
	go ds.handleDiscoveredPeers()
	go ds.handlePeerDiscoveryRequest()
	go ds.discover()
}
