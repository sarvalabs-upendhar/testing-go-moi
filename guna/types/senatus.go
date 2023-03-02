package types

import (
	"math"
	"sync"

	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
)

type NodeMetaInfoMsg struct {
	KramaID       id.KramaID
	Address       []string
	NTQ           float32
	WalletCount   int32
	PeerSignature []byte
}

func (miMsg *NodeMetaInfoMsg) HelloMessageBytes() ([]byte, error) {
	msg := ptypes.HelloMsg{
		KramaID:   miMsg.KramaID,
		Address:   miMsg.Address,
		Signature: nil,
	}

	return msg.Bytes()
}

func (miMsg *NodeMetaInfoMsg) NodeMetaInfo() *NodeMetaInfo {
	return &NodeMetaInfo{
		Addrs:         miMsg.Address,
		NTQ:           miMsg.NTQ,
		WalletCount:   miMsg.WalletCount,
		PeerSignature: miMsg.PeerSignature,
	}
}

type NodeMetaInfo struct {
	mtx           sync.RWMutex
	Addrs         []string
	NTQ           float32
	WalletCount   int32
	PublicKey     []byte
	PeerSignature []byte
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

func (mi *NodeMetaInfo) UpdatePublicKey(publicKey []byte) {
	mi.mtx.Lock()
	defer mi.mtx.Unlock()

	mi.PublicKey = publicKey
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
		return nil, errors.New("address not found")
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
	mi.mtx.Lock()
	// defer mi.mtx.Unlock()

	if err := polo.Depolorize(mi, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize reputation info")
	}

	return nil
}

type RequestQueue struct {
	mtx       sync.RWMutex
	elems     []*NodeMetaInfoMsg
	keys      map[id.KramaID]struct{}
	length    int
	maxLength int
}

func NewRequestQueue(maxSize int) *RequestQueue {
	return &RequestQueue{
		elems:     make([]*NodeMetaInfoMsg, 0, maxSize),
		keys:      make(map[id.KramaID]struct{}, maxSize),
		length:    0,
		maxLength: maxSize,
	}
}

func (q *RequestQueue) Push(element *NodeMetaInfoMsg) error {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	if element == nil {
		return nil
	}

	if q.length >= q.maxLength {
		return errors.New("queue is full")
	}

	q.elems = append(q.elems, element)

	q.length++
	q.keys[element.KramaID] = struct{}{}

	return nil
}

func (q *RequestQueue) Pop(count int) []*NodeMetaInfoMsg {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	if q.length > 0 {
		index := int(math.Min(float64(count), float64(q.length)))
		out := q.elems[:index]
		q.elems = q.elems[index:]

		q.length -= index

		for _, msg := range out {
			delete(q.keys, msg.KramaID)
		}

		return out
	}

	return nil
}

func (q *RequestQueue) Len() int {
	q.mtx.RLock()
	defer q.mtx.RUnlock()

	return q.length
}

func (q *RequestQueue) Contains(id id.KramaID) bool {
	q.mtx.RLock()
	defer q.mtx.RUnlock()

	_, ok := q.keys[id]

	return ok
}
