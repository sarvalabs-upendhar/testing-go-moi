package senatus

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/common"
)

type NodeMetaInfoMsg struct {
	KramaID       kramaid.KramaID
	Address       []string
	NTQ           float32
	WalletCount   int32
	PeerSignature []byte
}

type NodeMetaInfo struct {
	mtx           sync.RWMutex
	Addrs         []string
	KramaID       kramaid.KramaID
	NTQ           float32
	RTT           int64
	WalletCount   int32
	PeerSignature []byte
	Registered    bool `polo:"-"`
}

func (mi *NodeMetaInfo) UpdateNTQ(ntq float32) {
	mi.mtx.Lock()
	defer mi.mtx.Unlock()

	mi.NTQ = ntq
}

func (mi *NodeMetaInfo) UpdateWalletCount(delta int32) {
	mi.mtx.Lock()
	defer mi.mtx.Unlock()

	mi.WalletCount += delta
}

func (mi *NodeMetaInfo) GetKramaID() kramaid.KramaID {
	return mi.KramaID
}

func (mi *NodeMetaInfo) GetNTQ() float32 {
	mi.mtx.RLock()
	defer mi.mtx.RUnlock()

	return mi.NTQ
}

func (mi *NodeMetaInfo) GetWalletCount() int32 {
	mi.mtx.RLock()
	defer mi.mtx.RUnlock()

	return mi.WalletCount
}

func (mi *NodeMetaInfo) GetMultiAddress() ([]multiaddr.Multiaddr, error) {
	mi.mtx.RLock()
	defer mi.mtx.RUnlock()

	if len(mi.Addrs) == 0 {
		return nil, common.ErrAddressNotFound
	}

	multiAddrs := make([]multiaddr.Multiaddr, 0, len(mi.Addrs))

	for _, addr := range mi.Addrs {
		mAddr, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, err
		}

		multiAddrs = append(multiAddrs, mAddr)
	}

	return multiAddrs, nil
}

func (mi *NodeMetaInfo) Bytes() ([]byte, error) {
	mi.mtx.RLock()
	defer mi.mtx.RUnlock()

	rawData, err := polo.Polorize(mi)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize reputation info")
	}

	return rawData, nil
}

func (mi *NodeMetaInfo) FromBytes(bytes []byte) error {
	if err := polo.Depolorize(mi, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize reputation info")
	}

	return nil
}

type PeerInfo struct {
	ID   peer.ID
	Data []byte
}

func (pi *PeerInfo) Bytes() ([]byte, error) {
	rawData, err := polo.Polorize(pi)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize peer info")
	}

	return rawData, nil
}
