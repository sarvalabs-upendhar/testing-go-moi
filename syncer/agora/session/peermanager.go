package session

import (
	"context"
	"errors"
	"math/rand"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/sarvalabs/go-moi/syncer/agora/message"
)

type PeerManager struct {
	sessionID      identifiers.Address
	logger         hclog.Logger
	mtx            sync.Mutex
	peers          map[kramaid.KramaID]*PeerInfo
	connectedPeers map[kramaid.KramaID]interface{}
	network        sessionNetwork
}

type PeerInfo struct {
	failedAttempts int
	isActive       bool
	resp           chan bool
}

func NewSessionPeerManager(addr identifiers.Address, logger hclog.Logger, network sessionNetwork) *PeerManager {
	return &PeerManager{
		sessionID:      addr,
		logger:         logger.Named("Peer-Manager"),
		peers:          make(map[kramaid.KramaID]*PeerInfo),
		connectedPeers: make(map[kramaid.KramaID]interface{}),
		network:        network,
	}
}

func (spm *PeerManager) PeerRespChan(peerID kramaid.KramaID) <-chan bool {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	peerInfo, ok := spm.peers[peerID]
	if !ok {
		return nil
	}

	return peerInfo.resp
}

func (spm *PeerManager) peerConnected(peer kramaid.KramaID) {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	spm.connectedPeers[peer] = true
}

func (spm *PeerManager) PeerDisconnected(id peer.ID) {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	for kramaID := range spm.connectedPeers {
		peerID, err := utils.GetNetworkID(kramaID)
		if err != nil {
			continue
		}

		if peerID == id {
			delete(spm.connectedPeers, kramaID)
		}
	}
}

func (spm *PeerManager) AddPeers(peers ...kramaid.KramaID) {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	for _, peerID := range peers {
		spm.peers[peerID] = &PeerInfo{
			failedAttempts: 0,
			isActive:       false,
			resp:           make(chan bool),
		}
	}
}

func (spm *PeerManager) Signal(peerID kramaid.KramaID, status bool) error {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	info, ok := spm.peers[peerID]
	if !ok {
		return errors.New("peer not found")
	}

	info.resp <- status

	return nil
}

func (spm *PeerManager) PeerStatus(id kramaid.KramaID) bool {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	info, ok := spm.peers[id]
	if !ok {
		return false
	}

	return info.isActive
}

func (spm *PeerManager) UpdatePeerStatus(id kramaid.KramaID, status bool) bool {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	info, ok := spm.peers[id]
	if !ok {
		return false
	}

	info.isActive = status

	return true
}

func (spm *PeerManager) UpdateFailedAttempts(peer kramaid.KramaID, delta int) bool {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	info, ok := spm.peers[peer]
	if !ok {
		return false
	}

	info.failedAttempts += delta

	return true
}

func (spm *PeerManager) chooseBestPeer(
	ctx context.Context,
	avoidPeers map[kramaid.KramaID]interface{},
) (kramaid.KramaID, error) {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	if count := len(spm.connectedPeers); count > 0 {
		rejectedPeer := make(map[kramaid.KramaID]bool)
		for len(rejectedPeer) < count {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			default:
				r := rand.Intn(count)

				for peerID := range spm.connectedPeers {
					if _, ok := avoidPeers[peerID]; !ok && !rejectedPeer[peerID] {
						if r == 0 {
							info := spm.peers[peerID]
							if !info.isActive && info.failedAttempts < 3 {
								return peerID, nil
							} else {
								rejectedPeer[peerID] = true
							}
						}
					} else {
						rejectedPeer[peerID] = true
					}

					r--
				}
			}
		}
	}

	if count := len(spm.peers); count > 0 {
		rejectedPeer := make(map[kramaid.KramaID]bool)
		for len(rejectedPeer) < count {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			default:
				r := rand.Intn(count)

				for peerID, info := range spm.peers {
					if _, ok := avoidPeers[peerID]; !ok && !rejectedPeer[peerID] {
						if r == 0 {
							if !info.isActive && info.failedAttempts < 3 {
								return peerID, nil
							} else {
								rejectedPeer[peerID] = true
							}
						}
					} else {
						rejectedPeer[peerID] = true
					}

					r--
				}
			}
		}
	}

	return "", common.ErrPeerNotAvailable
}

func (spm *PeerManager) SendWantReq(peer kramaid.KramaID, msg *message.AgoraRequestMsg) error {
	if err := spm.network.SendAgoraMessage(peer, networkmsg.AGORAREQ, msg); err != nil {
		return err
	}

	spm.peerConnected(peer)

	return nil
}

func (spm *PeerManager) Close() {
	for kramaID := range spm.connectedPeers {
		if err := spm.network.ClosePeerSession(kramaID, spm.sessionID); err != nil {
			spm.logger.Error("Error closing peer session", "err", err)
		}
	}
}
