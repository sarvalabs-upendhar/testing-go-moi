package session

import (
	"context"
	"errors"
	"math/rand"
	"sync"

	"gitlab.com/sarvalabs/moichain/utils"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	atypes "gitlab.com/sarvalabs/moichain/poorna/agora/types"
	"gitlab.com/sarvalabs/moichain/types"
)

type SessionPeerManager struct {
	sessionID      types.Address
	logger         hclog.Logger
	mtx            sync.Mutex
	peers          map[id.KramaID]*PeerInfo
	connectedPeers map[id.KramaID]interface{}
	network        sessionNetwork
}

type PeerInfo struct {
	failedAttempts int
	isActive       bool
	resp           chan bool
}

func NewSessionPeerManager(addr types.Address, logger hclog.Logger, network sessionNetwork) *SessionPeerManager {
	return &SessionPeerManager{
		sessionID:      addr,
		logger:         logger,
		peers:          make(map[id.KramaID]*PeerInfo),
		connectedPeers: make(map[id.KramaID]interface{}),
		network:        network,
	}
}

func (spm *SessionPeerManager) PeerRespChan(peerID id.KramaID) <-chan bool {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	peerInfo, ok := spm.peers[peerID]
	if !ok {
		return nil
	}

	return peerInfo.resp
}

func (spm *SessionPeerManager) peerConnected(peer id.KramaID) {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	spm.connectedPeers[peer] = true
}

func (spm *SessionPeerManager) PeerDisconnected(id peer.ID) {
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

func (spm *SessionPeerManager) AddPeers(peers ...id.KramaID) {
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

func (spm *SessionPeerManager) Signal(peerID id.KramaID, status bool) error {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	info, ok := spm.peers[peerID]
	if !ok {
		return errors.New("peer not found")
	}

	info.resp <- status

	return nil
}

func (spm *SessionPeerManager) PeerStatus(id id.KramaID) bool {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	info, ok := spm.peers[id]
	if !ok {
		return false
	}

	return info.isActive
}

func (spm *SessionPeerManager) UpdatePeerStatus(id id.KramaID, status bool) bool {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	info, ok := spm.peers[id]
	if !ok {
		return false
	}

	info.isActive = status

	return true
}

func (spm *SessionPeerManager) UpdateFailedAttempts(peer id.KramaID, delta int) bool {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	info, ok := spm.peers[peer]
	if !ok {
		return false
	}

	info.failedAttempts += delta

	return true
}

func (spm *SessionPeerManager) chooseBestPeer(
	ctx context.Context,
	avoidPeers map[id.KramaID]interface{},
) (id.KramaID, error) {
	spm.mtx.Lock()
	defer spm.mtx.Unlock()

	if count := len(spm.connectedPeers); count > 0 {
		rejectedPeer := make(map[id.KramaID]bool)
		for len(rejectedPeer) < count {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			default:
				r := rand.Intn(count) //nolint

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
						r--
					} else {
						rejectedPeer[peerID] = true
					}
				}
			}
		}
	}

	if count := len(spm.peers); count > 0 {
		rejectedPeer := make(map[id.KramaID]bool)
		for len(rejectedPeer) < count {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			default:
				r := rand.Intn(count) //nolint

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

	return "", types.ErrPeerNotAvailable
}

func (spm *SessionPeerManager) SendWantReq(peer id.KramaID, msg *atypes.AgoraRequestMsg) error {
	if err := spm.network.SendAgoraMessage(peer, types.AGORAREQ, msg); err != nil {
		return err
	}

	spm.peerConnected(peer)

	return nil
}

func (spm *SessionPeerManager) Close() {
	for kramaID := range spm.connectedPeers {
		if err := spm.network.ClosePeerSession(kramaID, spm.sessionID); err != nil {
			spm.logger.Error("Error closing peer session")
		}
	}
}
