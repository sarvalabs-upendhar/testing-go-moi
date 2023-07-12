package decision

import (
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/common/kramaid"
	"github.com/sarvalabs/moichain/syncer/agora/block"
)

type PeerList struct {
	mtx           sync.RWMutex
	lastUpdatedAt int64
	peers         map[kramaid.KramaID]struct{}
}

func NewPeerList() *PeerList {
	p := &PeerList{
		lastUpdatedAt: 0,
		peers:         make(map[kramaid.KramaID]struct{}),
	}

	return p
}

func (plist *PeerList) Peers() []kramaid.KramaID {
	plist.mtx.RLock()
	defer plist.mtx.RUnlock()

	ids := make([]kramaid.KramaID, 0, len(plist.peers))

	for k := range plist.peers {
		ids = append(ids, k)
	}

	return ids
}

func (plist *PeerList) LastUpdatedAt() int64 {
	plist.mtx.Lock()
	defer plist.mtx.Unlock()

	return plist.lastUpdatedAt
}

func (plist *PeerList) AddPeer(peerID kramaid.KramaID) {
	plist.mtx.Lock()
	defer plist.mtx.Unlock()

	if _, ok := plist.peers[peerID]; !ok {
		if len(plist.peers) == block.MaxPeerListSize {
			for peerID := range plist.peers {
				delete(plist.peers, peerID)

				break
			}
		}

		plist.peers[peerID] = struct{}{}
		plist.lastUpdatedAt = time.Now().UnixNano()
	}
}

func (plist *PeerList) Size() int {
	plist.mtx.RLock()
	defer plist.mtx.RUnlock()

	return len(plist.peers)
}

func (plist *PeerList) CanonicalPeerList() *CanonicalPeerList {
	cp := &CanonicalPeerList{
		Peers:         plist.Peers(),
		LastUpdatedAt: plist.LastUpdatedAt(),
	}

	return cp
}

type CanonicalPeerList struct {
	Peers         []kramaid.KramaID
	LastUpdatedAt int64
}

func (clist *CanonicalPeerList) PeerList() *PeerList {
	peerList := NewPeerList()
	for _, peerID := range clist.Peers {
		peerList.AddPeer(peerID)
	}

	return peerList
}

func (clist *CanonicalPeerList) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(clist)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize canonical peer list")
	}

	return rawData, nil
}

func (clist *CanonicalPeerList) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(clist, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize canonical peer list")
	}

	return nil
}
